// Package store 下载任务的 SQLite 持久化层：任务、消息项、文件记录与断点续传。
// 采用 modernc.org/sqlite（纯 Go，无需 CGO），启用 WAL 与外键级联。
package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	_ "modernc.org/sqlite" // 注册 "sqlite" 驱动

	"tdl-ui/internal/logging"
)

var logStore = logging.L("store")

// schemaVersion 当前数据库结构版本，配合 PRAGMA user_version 做手写迁移。
const schemaVersion = 4

// Store 封装单个 SQLite 连接（单进程单连接池，配合 WAL + busy_timeout 规避 Windows 锁竞争）。
type Store struct {
	db *sql.DB
}

// Task 任务主表行（对应 tasks 表）。
type Task struct {
	ID         string
	Label      string
	Dir        string
	ScriptName string
	// ScriptSrc 任务创建时的脚本源码快照（SCR-04：恢复时优先用快照，
	// 避免脚本文件事后被编辑导致同一任务前后两截规则不一致）
	ScriptSrc  string
	Template   string
	RewriteExt bool
	SkipSame   bool
	GroupMedia bool
	Status     string
	Error      string
	Total      int
	Finished   int
	Failed     int
	CreatedAt  string
	UpdatedAt  string
}

// Item 任务消息项（对应 task_items 表）：url 类型填 URL，selection 类型填对话与消息 ID。
type Item struct {
	ID         int64
	TaskID     string
	ItemType   string // "url" | "selection"
	URL        string
	DialogID   int64
	DialogType string
	MessageIDs []int
	CreatedAt  string
}

// File 任务内文件记录（对应 files 表）。
type File struct {
	ID        int64
	TaskID    string
	Name      string
	Path      string
	Size      int64
	State     string // downloading / done / failed
	DialogID  int64
	MessageID int
	CreatedAt string
	UpdatedAt string
}

const (
	ItemTypeURL       = "url"
	ItemTypeSelection = "selection"
)

// Open 打开（或新建）数据库，执行连接级 PRAGMA 与结构迁移。
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, errors.Wrap(err, "创建数据目录失败")
	}

	// 连接级 PRAGMA 通过 DSN 注入，确保每条连接都生效。
	// STO-04：WAL 模式下 synchronous=NORMAL 安全（掉电最多丢最近事务，不损坏库），写入显著更快。
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "打开数据库失败")
	}
	// 单连接：SQLite 写入串行化，避免 modernc 连接池下 WAL 竞争。
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// migrate 依据 user_version 递增建表。
func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return errors.Wrap(err, "读取 user_version 失败")
	}
	if version >= schemaVersion {
		return nil
	}

	if version < 1 {
		if err := s.migrateV1(); err != nil {
			return errors.Wrap(err, "迁移至 v1 失败")
		}
	}
	if version < 2 {
		if err := s.migrateV2(); err != nil {
			return errors.Wrap(err, "迁移至 v2 失败")
		}
	}
	if version < 3 {
		if err := s.migrateV3(); err != nil {
			return errors.Wrap(err, "迁移至 v3 失败")
		}
	}
	if version < 4 {
		if err := s.migrateV4(); err != nil {
			return errors.Wrap(err, "迁移至 v4 失败")
		}
	}

	logStore.Infof("数据库结构已就绪，版本=%d", schemaVersion)
	return nil
}

func (s *Store) migrateV1() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id          TEXT PRIMARY KEY,
			label       TEXT,
			dir         TEXT NOT NULL,
			script_name TEXT,
			template    TEXT,
			rewrite_ext INTEGER NOT NULL DEFAULT 0,
			skip_same   INTEGER NOT NULL DEFAULT 0,
			group_media INTEGER NOT NULL DEFAULT 1,
			status      TEXT NOT NULL,
			error       TEXT,
			total       INTEGER NOT NULL DEFAULT 0,
			finished    INTEGER NOT NULL DEFAULT 0,
			failed      INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS task_items (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			item_type   TEXT NOT NULL,
			url         TEXT,
			dialog_id   INTEGER,
			dialog_type TEXT,
			message_ids TEXT,
			created_at  TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_items_url ON task_items(task_id, url) WHERE item_type = 'url'`,
		`CREATE TABLE IF NOT EXISTS files (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			name        TEXT NOT NULL,
			path        TEXT NOT NULL,
			size        INTEGER,
			state       TEXT NOT NULL,
			dialog_id   INTEGER,
			message_id  INTEGER,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			UNIQUE(task_id, path)
		)`,
		`CREATE TABLE IF NOT EXISTS resume_points (
			task_id     TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
			finished    TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_files_task ON files(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_items_task ON task_items(task_id)`,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return errors.Wrapf(err, "执行建表语句失败: %s", stmt)
		}
	}
	// 版本标记与结构变更在同一事务内提交，避免两者之间崩溃导致重跑迁移失败。
	if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
		return errors.Wrap(err, "写入 user_version 失败")
	}
	return tx.Commit()
}

// migrateV2 断点表由“每任务一行 JSON 集合”改为逐 key 行（ENG-12：
// 每文件完成只追加一行，避免重写整个集合的 O(n²) 写放大）。
func (s *Store) migrateV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS resume_keys (
		task_id    TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		resume_key TEXT NOT NULL,
		PRIMARY KEY (task_id, resume_key)
	) WITHOUT ROWID`); err != nil {
		return errors.Wrap(err, "创建 resume_keys 失败")
	}

	// 旧 JSON 集合数据展开为逐行（先读完再写，单连接下不可边遍历边执行）
	type legacyResume struct {
		taskID string
		keys   []string
	}
	var legacy []legacyResume
	rows, err := tx.Query(`SELECT task_id, finished FROM resume_points`)
	if err == nil {
		for rows.Next() {
			var taskID, raw string
			if err := rows.Scan(&taskID, &raw); err != nil {
				_ = rows.Close()
				return errors.Wrap(err, "读取旧断点失败")
			}
			var keys []string
			if raw != "" {
				if err := json.Unmarshal([]byte(raw), &keys); err != nil {
					logStore.Warnf("任务 %s 旧断点解析失败，跳过迁移: %v", taskID, err)
					continue
				}
			}
			legacy = append(legacy, legacyResume{taskID: taskID, keys: keys})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return errors.Wrap(err, "遍历旧断点失败")
		}
		_ = rows.Close()
		for _, lr := range legacy {
			for _, k := range lr.keys {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO resume_keys (task_id, resume_key) VALUES (?,?)`, lr.taskID, k); err != nil {
					return errors.Wrap(err, "迁移断点失败")
				}
			}
		}
		if _, err := tx.Exec(`DROP TABLE resume_points`); err != nil {
			return errors.Wrap(err, "删除旧断点表失败")
		}
	}

	if _, err := tx.Exec("PRAGMA user_version = 2"); err != nil {
		return errors.Wrap(err, "写入 user_version 失败")
	}
	return tx.Commit()
}

// migrateV3 tasks 表新增 script_src 列：任务创建时存入脚本源码快照，
// 恢复时优先用快照而非按名重载，避免脚本被编辑后语义漂移（SCR-04）。
func (s *Store) migrateV3() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`ALTER TABLE tasks ADD COLUMN script_src TEXT NOT NULL DEFAULT ''`); err != nil {
		return errors.Wrap(err, "新增 script_src 列失败")
	}
	if _, err := tx.Exec("PRAGMA user_version = 3"); err != nil {
		return errors.Wrap(err, "写入 user_version 失败")
	}
	return tx.Commit()
}

// migrateV4 files 表新增 (dialog_id, message_id) 索引：
// 对话媒体页按对话查询已下载文件（"已下载"标记）走索引而非全表扫。
func (s *Store) migrateV4() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_files_dialog_msg ON files(dialog_id, message_id)`); err != nil {
		return errors.Wrap(err, "创建 idx_files_dialog_msg 索引失败")
	}
	if _, err := tx.Exec("PRAGMA user_version = 4"); err != nil {
		return errors.Wrap(err, "写入 user_version 失败")
	}
	return tx.Commit()
}

// ---- JSON 编解码辅助（message_ids 与断点集合以 JSON 文本存列） ----

func marshalIntSlice(v []int) (string, error) {
	if len(v) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func unmarshalIntSlice(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	var v []int
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
