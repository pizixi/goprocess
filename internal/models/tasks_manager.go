package models

import (
	"context"
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
)

type TaskManager struct {
	tasks     []Task
	taskMutex sync.RWMutex
	cronJob   *cron.Cron
	db        Database
}

func NewTaskManager(db Database, cronJob *cron.Cron) *TaskManager {
	return &TaskManager{
		db:      db,
		cronJob: cronJob,
	}
}

func (tm *TaskManager) LoadTasks(ctx context.Context) error {
	tm.taskMutex.Lock()
	defer tm.taskMutex.Unlock()

	return tm.db.Find(ctx, &tm.tasks)
}

func (tm *TaskManager) GetEnabledTasks() []Task {
	tm.taskMutex.RLock()
	defer tm.taskMutex.RUnlock()

	var enabledTasks []Task
	for _, task := range tm.tasks {
		if task.IsEnabled {
			enabledTasks = append(enabledTasks, task)
		}
	}
	return enabledTasks
}

func (tm *TaskManager) GetAllTasks() []Task {
	tm.taskMutex.RLock()
	defer tm.taskMutex.RUnlock()

	tasksCopy := make([]Task, len(tm.tasks))
	copy(tasksCopy, tm.tasks)
	return tasksCopy
}

func (tm *TaskManager) AddTask(ctx context.Context, task *Task) error {
	tm.taskMutex.Lock()
	defer tm.taskMutex.Unlock()
	// 判断定时表达式是否合法
	if _, err := cron.ParseStandard(task.Cron); err != nil {
		return fmt.Errorf("定时表达式不合法: %w", err)
	}

	if err := tm.db.Create(ctx, task); err != nil {
		return err
	}

	tm.tasks = append(tm.tasks, *task)
	return nil
}

func (tm *TaskManager) UpdateTask(ctx context.Context, task *Task) error {
	tm.taskMutex.Lock()
	defer tm.taskMutex.Unlock()
	// 判断定时表达式是否合法
	if _, err := cron.ParseStandard(task.Cron); err != nil {
		return fmt.Errorf("定时表达式不合法: %w", err)
	}

	for i, t := range tm.tasks {
		if t.ID == task.ID {
			// 保持 IsEnabled 和 CronEntryID 不变
			task.IsEnabled = t.IsEnabled
			task.CronEntryID = t.CronEntryID
			tm.tasks[i] = *task

			if err := tm.db.Save(ctx, task); err != nil {
				return err
			}

			return nil
		}
	}

	return fmt.Errorf("任务未找到")
}

func (tm *TaskManager) DeleteTask(ctx context.Context, id uint) error {
	tm.taskMutex.Lock()
	defer tm.taskMutex.Unlock()

	for i, task := range tm.tasks {
		if task.ID == id {
			tm.cronJob.Remove(task.CronEntryID)
			tm.tasks = append(tm.tasks[:i], tm.tasks[i+1:]...)

			return tm.db.Delete(ctx, &task)
		}
	}

	return fmt.Errorf("任务未找到")
}

func (tm *TaskManager) GetTask(id uint) (*Task, bool) {
	tm.taskMutex.RLock()
	defer tm.taskMutex.RUnlock()

	for _, task := range tm.tasks {
		if task.ID == id {
			return &task, true
		}
	}

	return nil, false
}

func (tm *TaskManager) ToggleTaskStatus(ctx context.Context, id uint, isEnabled bool) error {
	tm.taskMutex.Lock()
	defer tm.taskMutex.Unlock()

	var task *Task
	for i, t := range tm.tasks {
		if t.ID == id {
			task = &tm.tasks[i]
			break
		}
	}

	if task == nil {
		return fmt.Errorf("任务未找到")
	}

	// 更新内存中的任务状态
	task.IsEnabled = isEnabled

	// 如果禁用任务，重置 CronEntryID
	if !isEnabled {
		task.CronEntryID = 0
	}

	// 更新数据库中的任务状态
	if err := tm.db.Save(ctx, &task); err != nil {
		return fmt.Errorf("更新数据库任务状态失败: %w", err)
	}

	return nil
}
