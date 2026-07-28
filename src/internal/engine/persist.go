package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TaskFile 任务内单个文件记录（下载中为 .tmp 临时路径，完成后为最终路径）。
type TaskFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	// State: downloading / done / failed
	State string `json:"state"`
}

// taskRecord 任务的可序列化快照，持久化于 <dataDir>/tasks.json。
type taskRecord struct {
	ID        string      `json:"id"`
	CreatedAt string      `json:"createdAt"`
	Opts      TaskOptions `json:"opts"`
	Status    string      `json:"status"`
	Error     string      `json:"error,omitempty"`
	Total     int         `json:"total"`
	Finished  int         `json:"finished"`
	Failed    int         `json:"failed"`
	Files     []TaskFile  `json:"files,omitempty"`
}

// loadRecords 读取任务记录；文件不存在视为空列表。
func loadRecords(path string) ([]taskRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var recs []taskRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// saveRecords 原子写任务记录（tmp + rename，避免中途崩溃损坏文件）。
func saveRecords(path string, recs []taskRecord) error {
	b, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
