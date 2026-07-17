package podcast

import (
	"os"
)

func (q *QueueManager) Pause(taskID string) {
	q.mu.RLock()
	task, exists := q.tasksMap[taskID]
	q.mu.RUnlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusDownloading {
			if task.cancelFunc != nil {
				task.cancelFunc()
			}
		} else if task.Status == StatusPending {
			task.Status = StatusPaused
		}
		task.mu.Unlock()
	}
}

func (q *QueueManager) Resume(taskID string) {
	q.mu.RLock()
	task, exists := q.tasksMap[taskID]
	q.mu.RUnlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusPaused || task.Status == StatusFailed {
			task.Status = StatusPending
			task.mu.Unlock()
			go q.processTask(task)
			return
		}
		task.mu.Unlock()
	}
}

func (q *QueueManager) Cancel(taskID string) {
	q.mu.Lock()
	task, exists := q.tasksMap[taskID]
	if exists {
		// Remove from slice
		for i, t := range q.tasks {
			if t.ID == taskID {
				q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
				break
			}
		}
		delete(q.tasksMap, taskID)
	}
	q.mu.Unlock()

	if exists {
		task.mu.Lock()
		if task.Status == StatusDownloading && task.cancelFunc != nil {
			task.cancelFunc()
		}
		task.mu.Unlock()
		// Clean up partial file
		_ = os.Remove(task.DestPath)
	}
}

func (q *QueueManager) CancelAll() {
	q.mu.Lock()
	tasksToCancel := make([]*DownloadTask, len(q.tasks))
	copy(tasksToCancel, q.tasks)
	q.tasks = nil
	q.tasksMap = make(map[string]*DownloadTask)
	q.mu.Unlock()

	for _, task := range tasksToCancel {
		task.mu.Lock()
		if task.Status == StatusDownloading && task.cancelFunc != nil {
			task.cancelFunc()
		}
		task.mu.Unlock()
		_ = os.Remove(task.DestPath)
	}
}
