package store

import (
	"context"

	"github.com/go-faster/errors"
)

// 断点表 resume_keys 为逐 key 行结构（ENG-12）：每文件完成仅追加一行，
// 替代旧版“每任务一行 JSON 集合”的全量重写。

// LoadFinished 读取任务断点集合（元素为 "dialogID:messageID"）。
func (s *Store) LoadFinished(ctx context.Context, taskID string) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT resume_key FROM resume_keys WHERE task_id=?`, taskID)
	if err != nil {
		return nil, errors.Wrap(err, "读取断点失败")
	}
	defer func() { _ = rows.Close() }()

	set := make(map[string]struct{})
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, errors.Wrap(err, "读取断点失败")
		}
		set[k] = struct{}{}
	}
	return set, rows.Err()
}

// AddFinished 追加单个断点 key（每文件完成时调用，O(1) 行级写入）。
func (s *Store) AddFinished(ctx context.Context, taskID, key string) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO resume_keys (task_id, resume_key) VALUES (?,?)`, taskID, key)
	if err != nil {
		return errors.Wrap(err, "保存断点失败")
	}
	return nil
}

// SaveFinished 批量补写断点集合（任务中断时兜底持久化，幂等）。
func (s *Store) SaveFinished(ctx context.Context, taskID string, finished map[string]struct{}) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for k := range finished {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO resume_keys (task_id, resume_key) VALUES (?,?)`, taskID, k); err != nil {
			return errors.Wrap(err, "保存断点失败")
		}
	}
	return tx.Commit()
}

// DeleteResume 删除任务断点行（Restart 或任务成功完成时调用）。
func (s *Store) DeleteResume(ctx context.Context, taskID string) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM resume_keys WHERE task_id=?`, taskID)
	return err
}
