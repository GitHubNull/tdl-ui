package store

import (
	"context"
	"database/sql"

	"github.com/go-faster/errors"
)

// UpsertFile 插入或按 (task_id, path) 覆盖文件记录（对应内存 addFile 的同路径覆盖语义）。
func (s *Store) UpsertFile(ctx context.Context, f File) error {
	now := nowStr()
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO files
		(task_id, name, path, size, state, dialog_id, message_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(task_id, path) DO UPDATE SET
			name=excluded.name, size=excluded.size, state=excluded.state,
			dialog_id=excluded.dialog_id, message_id=excluded.message_id, updated_at=excluded.updated_at`,
		f.TaskID, f.Name, f.Path, f.Size, f.State, f.DialogID, f.MessageID, f.CreatedAt, now)
	if err != nil {
		return errors.Wrap(err, "写入文件记录失败")
	}
	return nil
}

// FinishFile 把 oldPath（.tmp）行替换为最终文件记录：
// 同事务内先删可能已存在的最终路径旧行，再改写 .tmp 行的路径（对齐内存 finishFile 语义）。
func (s *Store) FinishFile(ctx context.Context, taskID, oldPath string, f File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := nowStr()
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	// 删除已存在的最终路径行（避免 UNIQUE 冲突），但不误删 oldPath 自身。
	if f.Path != oldPath {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE task_id=? AND path=?`, taskID, f.Path); err != nil {
			return errors.Wrap(err, "清理最终路径旧行失败")
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE files SET name=?, path=?, size=?, state=?, dialog_id=?, message_id=?, updated_at=?
		WHERE task_id=? AND path=?`,
		f.Name, f.Path, f.Size, f.State, f.DialogID, f.MessageID, now, taskID, oldPath)
	if err != nil {
		return errors.Wrap(err, "改写文件记录失败")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// oldPath 行不存在（历史遗留），直接插入最终行。
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO files
			(task_id, name, path, size, state, dialog_id, message_id, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			taskID, f.Name, f.Path, f.Size, f.State, f.DialogID, f.MessageID, f.CreatedAt, now); err != nil {
			return errors.Wrap(err, "插入最终文件记录失败")
		}
	}
	return tx.Commit()
}

// MarkFileFailed 标记文件状态为 failed。
func (s *Store) MarkFileFailed(ctx context.Context, taskID, path string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE files SET state='failed', updated_at=? WHERE task_id=? AND path=?`,
		nowStr(), taskID, path)
	return err
}

// DropFile 移除文件记录（临时文件已清理）。
func (s *Store) DropFile(ctx context.Context, taskID, path string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE task_id=? AND path=?`, taskID, path)
	return err
}

// ListFiles 返回任务的全部文件记录。
func (s *Store) ListFiles(ctx context.Context, taskID string) ([]File, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, name, path, size, state, dialog_id, message_id, created_at, updated_at
		FROM files WHERE task_id=? ORDER BY id ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var files []File
	for rows.Next() {
		var f File
		var size sql.NullInt64
		var dialogID sql.NullInt64
		var messageID sql.NullInt64
		if err := rows.Scan(&f.ID, &f.TaskID, &f.Name, &f.Path, &size, &f.State,
			&dialogID, &messageID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Size = size.Int64
		f.DialogID = dialogID.Int64
		f.MessageID = int(messageID.Int64)
		files = append(files, f)
	}
	return files, rows.Err()
}

// DeleteFilesByPath 删除任务内指定路径的文件记录。
func (s *Store) DeleteFilesByPath(ctx context.Context, taskID string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, p := range paths {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE task_id=? AND path=?`, taskID, p); err != nil {
			return err
		}
	}
	return tx.Commit()
}
