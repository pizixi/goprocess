package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	switch runtime.GOOS {
	case "windows":
		batFilePath := filepath.Join(task.WorkDir, fmt.Sprintf("task_%d.bat", task.ID))
		err := writeCommandToBatFile(batFilePath, task.Command)
		if err != nil {
			log.Printf("为任务 %v 创建批处理文件时出错: %v", task.ID, err)
		}
		defer os.Remove(batFilePath) // 运行完成后删除.bat脚本文件
		cmd = kexec.CommandString(fmt.Sprintf("task_%d.bat", task.ID))
	default:
		cmd = kexec.CommandString(task.Command)
	}
	cmd.Dir = task.WorkDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("为任务 %v 创建日志目录时出错: %v", task.ID, err)
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
	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 开始: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(logFile, "执行任务时出错: %v\n", err)
	}
	// 在任务结束时写入提示信息
	fmt.Fprintf(logFile, "---任务ID: %d 任务名: %s 结束: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))
}

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

// 	// 创建一个带缓冲的writer
// 	bufferedWriter := bufio.NewWriter(logFile)

// 	// 创建一个io.Writer，在每次写入后都会刷新缓冲区
// 	writer := &flushWriter{bufferedWriter}

// 	// 在任务开始时写入提示信息
// 	fmt.Fprintf(writer, "---任务ID: %d 任务名: %s 开始运行于: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))

// 	cmd.Stdout = writer
// 	cmd.Stderr = writer

// 	err := cmd.Run()
// 	if err != nil {
// 		fmt.Fprintf(writer, "执行任务时出错: %v\n", err)
// 	}

// 	// 在任务结束时写入提示信息
// 	fmt.Fprintf(writer, "---任务ID: %d 任务名: %s 结束运行于: %s---\n", task.ID, task.Name, time.Now().Format("2006-01-02 15:04:05"))

// 	// 确保所有数据都被写入
// 	bufferedWriter.Flush()
// }

// // flushWriter 是一个自定义的 io.Writer，它在每次写入后都会刷新缓冲区
// type flushWriter struct {
// 	w *bufio.Writer
// }

// func (fw *flushWriter) Write(p []byte) (n int, err error) {
// 	n, err = fw.w.Write(p)
// 	if err != nil {
// 		return
// 	}
// 	err = fw.w.Flush()
// 	return
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
