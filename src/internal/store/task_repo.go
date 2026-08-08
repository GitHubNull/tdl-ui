package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-faster/errors"
)

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

// InsertTask 在事务中插入任务主表 + 消息项。
func (s *Store) InsertTask(ctx context.Context, t Task, items []Item) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := nowStr()
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks
		(id, label, dir, script_name, script_src, template, renames, rewrite_ext, skip_same, group_media, status, error, total, finished, failed, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Label, t.Dir, t.ScriptName, t.ScriptSrc, t.Template, t.Renames,
		boolToInt(t.RewriteExt), boolToInt(t.SkipSame), boolToInt(t.GroupMedia),
		t.Status, t.Error, t.Total, t.Finished, t.Failed, t.CreatedAt, now); err != nil {
		return errors.Wrap(err, "插入任务失败")
	}

	for _, it := range items {
		if err := insertItemTx(ctx, tx, t.ID, it); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertItemTx(ctx context.Context, tx *sql.Tx, taskID string, it Item) error {
	ids, err := marshalIntSlice(it.MessageIDs)
	if err != nil {
		return err
	}
	var dialogID any
	if it.ItemType == ItemTypeSelection {
		dialogID = it.DialogID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_items
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
func (s *Store) UpdateTaskStatus(ctx context.Context, id, status, errMsg string) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET status=?, error=?, updated_at=? WHERE id=?`,
		status, errMsg, nowStr(), id)
	return err
}

// UpdateTaskState 单条 UPDATE 同步状态与计数（ENG-10：避免两条独立 UPDATE
// 在多 worker 并发下乱序写入时状态与计数撕裂）。
func (s *Store) UpdateTaskState(ctx context.Context, id, status, errMsg string, total, finished, failed int) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET status=?, error=?, total=?, finished=?, failed=?, updated_at=? WHERE id=?`,
		status, errMsg, total, finished, failed, nowStr(), id)
	return err
}

// DeleteTask 删除任务（外键级联清理 items/files/resume_keys）。
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id)
	return err
}

// LoadAllTasks 按创建顺序返回全部任务主表行。
func (s *Store) LoadAllTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, dir, script_name, script_src, template, renames,
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
		var label, script, src, tpl, renames, errMsg sql.NullString
		if err := rows.Scan(&t.ID, &label, &t.Dir, &script, &src, &tpl, &renames,
			&rewrite, &skip, &group, &t.Status, &errMsg,
			&t.Total, &t.Finished, &t.Failed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Label = label.String
		t.ScriptName = script.String
		t.ScriptSrc = src.String
		t.Template = tpl.String
		t.Renames = renames.String
		t.Error = errMsg.String
		t.RewriteExt = rewrite != 0
		t.SkipSame = skip != 0
		t.GroupMedia = group != 0
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListItems 返回任务的全部消息项。
func (s *Store) ListItems(ctx context.Context, taskID string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, item_type, url, dialog_id, dialog_type, message_ids, created_at
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
