package store

import (
	"encoding/json"
	"os"

	"github.com/go-faster/errors"
)

// 旧版 tasks.json 的可反序列化结构（仅用于一次性导入，字段与 engine.taskRecord 对齐）。
type legacyTaskFile struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	State string `json:"state"`
}

type legacySelection struct {
	DialogID   int64  `json:"dialogId"`
	DialogType string `json:"dialogType"`
	MessageIDs []int  `json:"messageIds"`
}

type legacyOpts struct {
	URLs       []string          `json:"urls"`
	Selections []legacySelection `json:"selections"`
	Label      string            `json:"label"`
	Dir        string            `json:"dir"`
	ScriptName string            `json:"scriptName"`
	Template   string            `json:"template"`
	RewriteExt bool              `json:"rewriteExt"`
	SkipSame   bool              `json:"skipSame"`
	Group      bool              `json:"group"`
	Restart    bool              `json:"restart"`
}

type legacyRecord struct {
	ID        string           `json:"id"`
	CreatedAt string           `json:"createdAt"`
	Opts      legacyOpts       `json:"opts"`
	Status    string           `json:"status"`
	Error     string           `json:"error,omitempty"`
	Total     int              `json:"total"`
	Finished  int              `json:"finished"`
	Failed    int              `json:"failed"`
	Files     []legacyTaskFile `json:"files,omitempty"`
}

// ImportLegacyJSON 首次运行时把旧 tasks.json 导入三表：
// 仅在 tasks 表为空且 tasks.json 存在时执行；成功后原文件重命名为 .bak。
// running/queued 状态降级为 paused（可断点续传）。旧 BBolt 断点不迁移。
func (s *Store) ImportLegacyJSON(tasksJSONPath string) error {
	// tasks 表非空说明已初始化过，跳过。
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM tasks`).Scan(&count); err != nil {
		return errors.Wrap(err, "统计任务数失败")
	}
	if count > 0 {
		return nil
	}

	b, err := os.ReadFile(tasksJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 无旧数据，全新启动
		}
		return errors.Wrap(err, "读取旧任务记录失败")
	}

	var recs []legacyRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		return errors.Wrap(err, "解析旧任务记录失败")
	}
	if len(recs) == 0 {
		return s.renameLegacy(tasksJSONPath)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, r := range recs {
		status := r.Status
		if status == "running" || status == "queued" {
			status = "paused"
		}
		created := r.CreatedAt
		if created == "" {
			created = nowStr()
		}
		if _, err := tx.Exec(`INSERT INTO tasks
			(id, label, dir, script_name, template, rewrite_ext, skip_same, group_media, status, error, total, finished, failed, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.ID, r.Opts.Label, r.Opts.Dir, r.Opts.ScriptName, r.Opts.Template,
			boolToInt(r.Opts.RewriteExt), boolToInt(r.Opts.SkipSame), boolToInt(r.Opts.Group),
			status, r.Error, r.Total, r.Finished, r.Failed, created, nowStr()); err != nil {
			return errors.Wrapf(err, "导入任务 %s 失败", r.ID)
		}

		for _, u := range r.Opts.URLs {
			if err := insertItemTx(tx, r.ID, Item{ItemType: ItemTypeURL, URL: u}); err != nil {
				return err
			}
		}
		for _, sel := range r.Opts.Selections {
			if err := insertItemTx(tx, r.ID, Item{
				ItemType:   ItemTypeSelection,
				DialogID:   sel.DialogID,
				DialogType: sel.DialogType,
				MessageIDs: sel.MessageIDs,
			}); err != nil {
				return err
			}
		}
		for _, f := range r.Files {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO files
				(task_id, name, path, size, state, dialog_id, message_id, created_at, updated_at)
				VALUES (?,?,?,?,?,?,?,?,?)`,
				r.ID, f.Name, f.Path, f.Size, f.State, 0, 0, created, nowStr()); err != nil {
				return errors.Wrapf(err, "导入任务 %s 文件失败", r.ID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	logStore.Infof("已从 tasks.json 导入 %d 条任务记录", len(recs))
	return s.renameLegacy(tasksJSONPath)
}

func (s *Store) renameLegacy(path string) error {
	if err := os.Rename(path, path+".bak"); err != nil && !os.IsNotExist(err) {
		logStore.Warnf("重命名旧任务记录失败: %v", err)
	}
	return nil
}
