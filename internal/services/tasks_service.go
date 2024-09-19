package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/robfig/cron/v3"
)

type TaskService struct {
	cronJob      *cron.Cron
	taskMutex    sync.RWMutex
	enabledTasks map[uint]*models.Task
}

func NewTaskService(cronJob *cron.Cron) *TaskService {
	return &TaskService{
		cronJob:      cronJob,
		enabledTasks: make(map[uint]*models.Task),
	}
}

func (s *TaskService) ScheduleTask(task *models.Task) error {
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()

	if existingTask, exists := s.enabledTasks[task.ID]; exists {
		if existingTask.CronEntryID != 0 {
			log.Printf("正在移除任务 %v 的现有 CronEntryID: %v", task.ID, existingTask.CronEntryID)
			s.cronJob.Remove(existingTask.CronEntryID)
		}
	}

	entryID, err := s.cronJob.AddFunc(task.Cron, func() {
		ExecuteTask(task)
	})
	if err != nil {
		log.Printf("为任务 %v 添加 cron 函数时出错: %v", task.ID, err)
		return err
	}

	task.CronEntryID = entryID
	s.enabledTasks[task.ID] = task
	log.Printf("已安排任务 %v 名称 %s，cron 条目: %v", task.ID, task.Name, entryID)

	go func() {
		s.PrintEnabledTasks()
	}()

	return nil
}

func (s *TaskService) UnscheduleTask(task *models.Task) {
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()
	if existingTask, exists := s.enabledTasks[task.ID]; exists {
		if existingTask.CronEntryID != 0 {
			log.Printf("正在移除任务 %v 的现有 CronEntryID: %v", task.ID, existingTask.CronEntryID)
			s.cronJob.Remove(existingTask.CronEntryID)
			task.CronEntryID = 0
			delete(s.enabledTasks, task.ID)
			log.Printf("已移除任务 %v 名称 %s 的定时任务", task.ID, task.Name)
			go func() {
				s.PrintEnabledTasks()
			}()
		}
	}

}

func (s *TaskService) PrintEnabledTasks() {
	s.taskMutex.RLock()
	defer s.taskMutex.RUnlock()

	log.Println("当前启用的定时任务：")
	for _, task := range s.enabledTasks {
		log.Printf("任务ID: %d, 名称: %s, Cron: %s,CronEntryID: %v \n", task.ID, task.Name, task.Cron, task.CronEntryID)
	}
}

func ExecuteTask(task *models.Task) {
	logDir := filepath.Join("logs", fmt.Sprintf("task_%d", task.ID))
	var cmd *kexec.KCommand

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "output.log"),
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
	// 使用同一个 logFile 实例
	// log.SetOutput(logFile)
	// 手动关闭日志文件，确保日志写入并归档
	defer logFile.Close()

	// 创建一个带有超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(task.Timeout)*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "windows":
		batFilePath := filepath.Join(task.WorkDir, fmt.Sprintf("task_%d.bat", task.ID))
		err := writeCommandToBatFile(batFilePath, task.Command)
		if err != nil {
			log.Printf("为任务 %v 创建批处理文件时出错: %v\n", task.ID, err)
		}
		defer os.Remove(batFilePath)
		cmd = kexec.CommandString(fmt.Sprintf("task_%d.bat", task.ID))
	default:
		cmd = kexec.CommandString(task.Command)
	}
	cmd.Dir = task.WorkDir

	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("为任务 %v 创建日志目录时出错: %v\n", task.ID, err)
		return
	}

	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 开始: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
	// log.Printf("---任务ID: %d 任务名: %s 开始: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// 在一个 goroutine 中运行命令
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// 等待命令完成或超时
	select {
	case <-ctx.Done():
		// 超时，杀死进程及其子进程
		if cmd.Process != nil {
			// killProcessGroup(cmd.Process.Pid)
			if runtime.GOOS == "windows" {
				cmd.Terminate(os.Kill)
			} else {
				cmd.Process.Signal(syscall.SIGKILL)
			}
		}
		// fmt.Fprintf(logFile, "任务超时，已强制终止\n")
		log.Printf("任务超时，已强制终止\n")
	case err := <-done:
		if err != nil {
			// fmt.Fprintf(logFile, "执行任务时出错: %v\n", err)
			log.Printf("执行任务时出错: %v\n", err)
		}
	}

	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 结束: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
	// log.Printf("---任务ID: %d 任务名: %s 结束: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))

}

// // killProcessGroup 杀死指定 PID 的进程及其所有子进程
// func killProcessGroup(pid int) {
// 	if runtime.GOOS == "windows" {
// 		exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
// 	} else {
// 		syscall.Kill(-pid, syscall.SIGKILL)
// 	}
// }

// func ExecuteTask(task *models.Task) {
// 	logDir := filepath.Join("logs", fmt.Sprintf("task_%d", task.ID))
// 	var cmd *kexec.KCommand
// 	switch runtime.GOOS {
// 	case "windows":
// 		batFilePath := filepath.Join(task.WorkDir, fmt.Sprintf("task_%d.bat", task.ID))
// 		err := writeCommandToBatFile(batFilePath, task.Command)
// 		if err != nil {
// 			log.Printf("为任务 %v 创建批处理文件时出错: %v", task.ID, err)
// 		}
// 		defer os.Remove(batFilePath) // 运行完成后删除.bat脚本文件
// 		cmd = kexec.CommandString(fmt.Sprintf("task_%d.bat", task.ID))
// 	default:
// 		cmd = kexec.CommandString(task.Command)
// 	}
// 	cmd.Dir = task.WorkDir
// 	if err := os.MkdirAll(logDir, 0755); err != nil {
// 		log.Printf("为任务 %v 创建日志目录时出错: %v", task.ID, err)
// 		return
// 	}
// 	logFile := &lumberjack.Logger{
// 		Filename:   filepath.Join(logDir, "output.log"),
// 		MaxSize:    10,
// 		MaxBackups: 3,
// 		MaxAge:     28,
// 		Compress:   true,
// 	}
// 	// 在任务开始时写入提示信息
// 	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 开始: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
// 	cmd.Stdout = logFile
// 	cmd.Stderr = logFile
// 	err := cmd.Run()
// 	if err != nil {
// 		fmt.Fprintf(logFile, "执行任务时出错: %v\n", err)
// 	}
// 	// 在任务结束时写入提示信息
// 	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 结束: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
// }

func writeCommandToBatFile(filePath string, command string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建批处理文件时出错: %w", err)
	}
	defer file.Close()

	// 根据换行符将命令拆分成多行
	lines := strings.Split(command, "\n")
	for _, line := range lines {
		// 写入每一行命令到.bat脚本文件
		_, err := file.WriteString(line + "\r\n") // 使用\r\n作为换行符
		if err != nil {
			return fmt.Errorf("写入批处理文件时出错: %w", err)
		}
	}

	return nil
}
