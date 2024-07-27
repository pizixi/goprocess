package models

import (
	"sync"

	"github.com/robfig/cron/v3"
)

//	type Task struct {
//		ID          string       `json:"id"`
//		Name        string       `json:"name"`
//		Cron        string       `json:"cron"`
//		WorkDir     string       `json:"workDir"`
//		Command     string       `json:"command"`
//		IsEnabled   bool         `json:"isEnabled"`
//		CronEntryID cron.EntryID `json:"-"` // 使用 `json:"-"` 防止序列化此字段
//	}
type Task struct {
	ID          uint         `json:"ID" gorm:"primaryKey"`
	Name        string       `json:"name"`
	Cron        string       `json:"cron"`
	WorkDir     string       `json:"workDir"`
	Command     string       `json:"command"`
	IsEnabled   bool         `json:"isEnabled"`
	CronEntryID cron.EntryID `json:"-" gorm:"-"` // 使用 `gorm:"-"` 防止存储此字段
}

var (
	Tasks     []Task
	TaskMutex sync.Mutex
	CronJob   *cron.Cron
)

// const tasksFile = "tasks.json"

// func ReadTasksFromJSON() error {
// 	file, err := os.ReadFile(tasksFile)
// 	if err != nil {
// 		if os.IsNotExist(err) {
// 			// 如果文件不存在,创建一个空的JSON文件
// 			return WriteTasksToJSON()
// 		}
// 		return err
// 	}

// 	if err := json.Unmarshal(file, &Tasks); err != nil {
// 		return err
// 	}
// 	return nil
// }

// func WriteTasksToJSON() error {
// 	data, err := json.MarshalIndent(Tasks, "", "  ")
// 	if err != nil {
// 		return err
// 	}

// 	return os.WriteFile(tasksFile, data, 0644)
// }

func GetAllTasks() ([]Task, error) {
	var tasks []Task
	result := DB.Find(&tasks)
	return tasks, result.Error
}

func CreateTask(task *Task) error {
	return DB.Create(task).Error
}

func UpdateTask(task *Task) error {
	return DB.Save(task).Error
}

func DeleteTask(id string) error {
	return DB.Delete(&Task{}, id).Error
}

func GetTaskByID(id string) (*Task, error) {
	var task Task
	result := DB.First(&task, "id = ?", id)
	return &task, result.Error
}
