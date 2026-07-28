package store

import (
	"database/sql"
	"time"

	"github.com/go-faster/errors"
)

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

// InsertTask 在事务中插入任务主表 + 消息项。
func (s *Store) InsertTask(t Task, items []Item) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := nowStr()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	if _, err := tx.Exec(`INSERT INTO tasks
		(id, label, dir, script_name, template, rewrite_ext, skip_same, group_media, status, error, total, finished, failed, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Label, t.Dir, t.ScriptName, t.Template,
		boolToInt(t.RewriteExt), boolToInt(t.SkipSame), boolToInt(t.GroupMedia),
		t.Status, t.Error, t.Total, t.Finished, t.Failed, t.CreatedAt, now); err != nil {
		return errors.Wrap(err, "插入任务失败")
	}

	for _, it := range items {
		if err := insertItemTx(tx, t.ID, it); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertItemTx(tx *sql.Tx, taskID string, it Item) error {
	ids, err := marshalIntSlice(it.MessageIDs)
	if err != nil {
		return err
	}
	var dialogID any
	if it.ItemType == ItemTypeSelection {
		dialogID = it.DialogID
	}
	if _, err := tx.Exec(`INSERT INTO task_items
		(task_id, item_type, url, dialog_id, dialog_type, message_ids, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		taskID, it.ItemType, nullString(it.URL), dialogID, nullString(it.DialogType), ids, nowStr()); err != nil {
		return errors.Wrap(err, "插入消息项失败")
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpdateTaskStatus 更新任务状态与错误信息。
func (s *Store) UpdateTaskStatus(id, status, errMsg string) error {
	_, err := s.db.Exec(`UPDATE tasks SET status=?, error=?, updated_at=? WHERE id=?`,
		status, errMsg, nowStr(), id)
	return err
}

// UpdateTaskCounts 更新任务进度计数。
func (s *Store) UpdateTaskCounts(id string, total, finished, failed int) error {
	_, err := s.db.Exec(`UPDATE tasks SET total=?, finished=?, failed=?, updated_at=? WHERE id=?`,
		total, finished, failed, nowStr(), id)
	return err
}

// DeleteTask 删除任务（外键级联清理 items/files/resume_points）。
func (s *Store) DeleteTask(id string) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

// LoadAllTasks 按创建顺序返回全部任务主表行。
func (s *Store) LoadAllTasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, label, dir, script_name, template,
		rewrite_ext, skip_same, group_media, status, error, total, finished, failed, created_at, updated_at
		FROM tasks ORDER BY created_at ASC, rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		var t Task
		var rewrite, skip, group int
		var label, script, tpl, errMsg sql.NullString
		if err := rows.Scan(&t.ID, &label, &t.Dir, &script, &tpl,
			&rewrite, &skip, &group, &t.Status, &errMsg,
			&t.Total, &t.Finished, &t.Failed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Label = label.String
		t.ScriptName = script.String
		t.Template = tpl.String
		t.Error = errMsg.String
		t.RewriteExt = rewrite != 0
		t.SkipSame = skip != 0
		t.GroupMedia = group != 0
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListItems 返回任务的全部消息项。
func (s *Store) ListItems(taskID string) ([]Item, error) {
	rows, err := s.db.Query(`SELECT id, task_id, item_type, url, dialog_id, dialog_type, message_ids, created_at
		FROM task_items WHERE task_id=? ORDER BY id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []Item
	for rows.Next() {
		var it Item
		var url, dialogType, ids sql.NullString
		var dialogID sql.NullInt64
		if err := rows.Scan(&it.ID, &it.TaskID, &it.ItemType, &url, &dialogID, &dialogType, &ids, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.URL = url.String
		it.DialogID = dialogID.Int64
		it.DialogType = dialogType.String
		msgIDs, err := unmarshalIntSlice(ids.String)
		if err != nil {
			return nil, err
		}
		it.MessageIDs = msgIDs
		items = append(items, it)
	}
	return items, rows.Err()
}

// AppendURLItem 追加一条 url 消息项；靠部分唯一索引 uq_items_url 去重，
// 重复 URL 静默忽略（返回 false 表示已存在未插入）。
func (s *Store) AppendURLItem(taskID, url string) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO task_items
		(task_id, item_type, url, message_ids, created_at) VALUES (?,?,?,?,?)`,
		taskID, ItemTypeURL, url, "[]", nowStr())
	if err != nil {
		return false, errors.Wrap(err, "追加 URL 项失败")
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MergeSelectionItem 追加/合并一条 selection 消息项：同 (task_id, dialog_id) 已存在时
// 求 message_ids 并集后更新，否则插入新行。
func (s *Store) MergeSelectionItem(taskID string, dialogID int64, dialogType string, messageIDs []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var rowID int64
	var existingJSON string
	err = tx.QueryRow(`SELECT id, message_ids FROM task_items
		WHERE task_id=? AND item_type=? AND dialog_id=?`,
		taskID, ItemTypeSelection, dialogID).Scan(&rowID, &existingJSON)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := insertItemTx(tx, taskID, Item{
			ItemType:   ItemTypeSelection,
			DialogID:   dialogID,
			DialogType: dialogType,
			MessageIDs: messageIDs,
		}); err != nil {
			return err
		}
	case err != nil:
		return errors.Wrap(err, "查询选集项失败")
	default:
		existing, err := unmarshalIntSlice(existingJSON)
		if err != nil {
			return err
		}
		merged := unionInts(existing, messageIDs)
		ids, err := marshalIntSlice(merged)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE task_items SET message_ids=? WHERE id=?`, ids, rowID); err != nil {
			return errors.Wrap(err, "合并选集项失败")
		}
	}
	return tx.Commit()
}

// unionInts 求两个 int 切片的并集，保持原顺序、去重。
func unionInts(a, b []int) []int {
	seen := make(map[int]struct{}, len(a)+len(b))
	out := make([]int, 0, len(a)+len(b))
	for _, v := range append(append([]int(nil), a...), b...) {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
