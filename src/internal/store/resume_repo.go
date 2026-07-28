package store

import (
	"database/sql"
	"encoding/json"

	"github.com/go-faster/errors"
)

// LoadFinished 读取任务断点集合（元素为 "dialogID:messageID"）。
func (s *Store) LoadFinished(taskID string) (map[string]struct{}, error) {
	var raw string
	err := s.db.QueryRow(`SELECT finished FROM resume_points WHERE task_id=?`, taskID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return make(map[string]struct{}), nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "读取断点失败")
	}

	var keys []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &keys); err != nil {
			return nil, errors.Wrap(err, "解析断点失败")
		}
	}
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set, nil
}

// SaveFinished upsert 任务断点集合。
func (s *Store) SaveFinished(taskID string, finished map[string]struct{}) error {
	keys := make([]string, 0, len(finished))
	for k := range finished {
		keys = append(keys, k)
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO resume_points (task_id, finished, updated_at)
		VALUES (?,?,?)
		ON CONFLICT(task_id) DO UPDATE SET finished=excluded.finished, updated_at=excluded.updated_at`,
		taskID, string(b), nowStr())
	if err != nil {
		return errors.Wrap(err, "保存断点失败")
	}
	return nil
}

// DeleteResume 删除任务断点行（Restart 或任务成功完成时调用）。
func (s *Store) DeleteResume(taskID string) error {
	_, err := s.db.Exec(`DELETE FROM resume_points WHERE task_id=?`, taskID)
	return err
}
