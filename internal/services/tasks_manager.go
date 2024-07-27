package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/robfig/cron/v3"
)

func InitializeCronJob() {
	models.CronJob = cron.New()
	models.CronJob.Start()
	InitializeEnabledTasks()
}

func InitializeEnabledTasks() {
	for i := range models.Tasks {
		task := &models.Tasks[i]
		if task.IsEnabled {
			ScheduleTask(task)
		}
	}
}

func CronJobCleanup() {
	if models.CronJob != nil {
		models.CronJob.Stop()
	}
}

func PrintActiveTasks() {
	entries := models.CronJob.Entries()
	fmt.Println("Current active tasks:")
	for _, entry := range entries {
		for _, task := range models.Tasks {
			if task.CronEntryID == entry.ID {
				fmt.Printf("- Task ID: %v, Name: %s, Cron: %s CronEntryID: %v\n", task.ID, task.Name, task.Cron, task.CronEntryID)
				break
			}
		}
	}
}

func ScheduleTask(task *models.Task) {
	if models.CronJob == nil {
		fmt.Println("Error: CronJob is not initialized")
		return
	}

	// 如果任务已经有一个 CronEntryID，先移除它
	if task.CronEntryID != 0 {
		models.CronJob.Remove(task.CronEntryID)
	}

	entryID, err := models.CronJob.AddFunc(task.Cron, func() { ExecuteTask(task) })
	if err != nil {
		fmt.Printf("Error scheduling task %v: %v\n", task.ID, err)
		return
	}
	task.CronEntryID = entryID
	fmt.Printf("Scheduled task %v with cron entry: %v\n", task.ID, entryID)

	// 更新 models.Tasks 中的相应任务
	for i, t := range models.Tasks {
		if t.ID == task.ID {
			models.Tasks[i] = *task
			break
		}
	}

	PrintActiveTasks()
}

func ExecuteTask(task *models.Task) {
	cmd := kexec.CommandString(task.Command)
	cmd.Dir = task.WorkDir

	// logFile, err := os.OpenFile(filepath.Join("logs", fmt.Sprintf("%d", task.ID)+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	fmt.Printf("Error opening log file for task %v: %v\n", task.ID, err)
	// 	return
	// }
	// defer logFile.Close()
	logDir := filepath.Join("logs", fmt.Sprintf("task_%d", task.ID))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Error creating log directory for task %v: %v", task.ID, err)
		return
	}

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "output.log"),
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
	// 在任务开始时写入提示信息
	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 开始运行于: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(logFile, "Error executing task: %v\n", err)
	}
	// 在任务结束时写入提示信息
	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 结束运行于: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
}
