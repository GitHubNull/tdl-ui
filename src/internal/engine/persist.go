package engine

import (
	"context"

	"tdl-ui/internal/store"
)

// 本文件收敛任务的 SQLite 持久化：store 行与内存结构的双向映射、状态/断点写回。

// loadFromStore 从 SQLite 恢复任务；上次退出时仍在运行/排队的任务恢复为已暂停（可断点续传）。
func (m *Manager) loadFromStore() {
	if m.deps.Store == nil { // 单测可能不提供 store
		return
	}
	// 启动阶段无调用方 ctx，且恢复必须完整完成（STO-03）。
	ctx := context.Background()
	tasks, err := m.deps.Store.LoadAllTasks(ctx)
	if err != nil {
		logEngine.Errorf("加载任务记录失败: %v", err)
		return
	}
	for _, st := range tasks {
		status := st.Status
		if status == StatusRunning || status == StatusQueued {
			status = StatusPaused
			_ = m.deps.Store.UpdateTaskStatus(ctx, st.ID, status, st.Error)
		}
		items, err := m.deps.Store.ListItems(ctx, st.ID)
		if err != nil {
			// ENG-13：降级加载而非静默丢弃——任务仍可见，状态标 failed 并附错误，
			// 用户至少能感知并手动删除；消息项置空，恢复执行时会再次报错。
			logEngine.Errorf("加载任务 %s 消息项失败: %v", st.ID, err)
			status = StatusFailed
			st.Error = "加载任务消息项失败: " + err.Error()
			items = nil
		}
		urls, selections := itemsToOptions(items)
		files, err := m.deps.Store.ListFiles(ctx, st.ID)
		if err != nil {
			logEngine.Errorf("加载任务 %s 文件失败: %v", st.ID, err)
		}
		t := &Task{
			ID:        st.ID,
			CreatedAt: st.CreatedAt,
			opts: TaskOptions{
				URLs:       urls,
				Selections: selections,
				Label:      st.Label,
				Dir:        st.Dir,
				ScriptName: st.ScriptName,
				Template:   st.Template,
				RewriteExt: st.RewriteExt,
				SkipSame:   st.SkipSame,
				Group:      st.GroupMedia,
			},
			mgr:       m,
			scriptSrc: st.ScriptSrc,
			status:    status,
			errMsg:    st.Error,
			total:     st.Total,
			finished:  st.Finished,
			failed:    st.Failed,
			files:     storeFilesToTaskFiles(files),
		}
		m.tasks[t.ID] = t
		m.order = append(m.order, t.ID)
	}
}

// itemsToOptions 把消息项行还原为 URLs / Selections。
func itemsToOptions(items []store.Item) ([]string, []Selection) {
	var urls []string
	var sels []Selection
	for _, it := range items {
		switch it.ItemType {
		case store.ItemTypeURL:
			if it.URL != "" {
				urls = append(urls, it.URL)
			}
		case store.ItemTypeSelection:
			sels = append(sels, Selection{
				DialogID:   it.DialogID,
				DialogType: it.DialogType,
				MessageIDs: it.MessageIDs,
			})
		}
	}
	return urls, sels
}

// optionsToItems 把任务选项拆成消息项行（用于插入）。
func optionsToItems(opts TaskOptions) []store.Item {
	items := make([]store.Item, 0, len(opts.URLs)+len(opts.Selections))
	for _, u := range opts.URLs {
		items = append(items, store.Item{ItemType: store.ItemTypeURL, URL: u})
	}
	for _, sel := range opts.Selections {
		items = append(items, store.Item{
			ItemType:   store.ItemTypeSelection,
			DialogID:   sel.DialogID,
			DialogType: sel.DialogType,
			MessageIDs: sel.MessageIDs,
		})
	}
	return items
}

// toStoreTask 把任务主字段转为 store.Task 行。
func toStoreTask(t *Task) store.Task {
	return store.Task{
		ID:         t.ID,
		Label:      t.opts.Label,
		Dir:        t.opts.Dir,
		ScriptName: t.opts.ScriptName,
		ScriptSrc:  t.scriptSrc,
		Template:   t.opts.Template,
		RewriteExt: t.opts.RewriteExt,
		SkipSame:   t.opts.SkipSame,
		GroupMedia: t.opts.Group,
		Status:     t.status,
		Error:      t.errMsg,
		Total:      t.total,
		Finished:   t.finished,
		Failed:     t.failed,
		CreatedAt:  t.CreatedAt,
	}
}

// storeFilesToTaskFiles 把文件表行转为内存文件记录。
func storeFilesToTaskFiles(files []store.File) []TaskFile {
	out := make([]TaskFile, 0, len(files))
	for _, f := range files {
		out = append(out, TaskFile{Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
			DialogID: f.DialogID, MessageID: f.MessageID})
	}
	return out
}

// persistState 行级写回任务状态与计数（单条 UPDATE，ENG-10：避免状态与计数
// 两条语句之间被并发旧快照乱序覆盖造成撕裂）。
func (t *Task) persistState() {
	if t.mgr.deps.Store == nil {
		return
	}
	t.mu.Lock()
	status, errMsg := t.status, t.errMsg
	total, finished, failed := t.total, t.finished, t.failed
	t.mu.Unlock()
	// 状态写回多发生在暂停/取消之后，不得随任务 ctx 中断（STO-03）。
	if err := t.mgr.deps.Store.UpdateTaskState(context.Background(), t.ID, status, errMsg, total, finished, failed); err != nil {
		logEngine.Errorf("更新任务状态失败: id=%s err=%v", t.ID, err)
	}
}

// saveResumeKey 追加持久化单个断点（由 progress.OnDone 每完成一个文件调用，
// ENG-12：O(1) 行级写入，替代旧版每次重写整个集合）。
func (t *Task) saveResumeKey(key string) {
	if t.mgr.deps.Store == nil {
		return
	}
	// 断点丢失代价是重复下载，不得随任务 ctx 中断（STO-03）。
	if err := t.mgr.deps.Store.AddFinished(context.Background(), t.ID, key); err != nil {
		logEngine.Errorf("保存断点失败: id=%s err=%v", t.ID, err)
	}
}
