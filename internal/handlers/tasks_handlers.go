package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/nxadm/tail"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/pkg/utils"
)

func GetTasksHandler(c echo.Context) error {
	response := map[string]interface{}{
		"code":  0,
		"msg":   "success",
		"count": len(models.Tasks),
		"data":  models.Tasks,
	}
	return c.JSON(http.StatusOK, response)
}

func CreateTaskHandler(c echo.Context) error {
	task := new(models.Task)
	if err := c.Bind(task); err != nil {
		return err
	}

	models.TaskMutex.Lock()
	defer models.TaskMutex.Unlock()

	result := models.DB.Create(&task)
	if result.Error != nil {
		log.Println(result.Error)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create task"})
	}
	// 获取插入数据的ID
	log.Println("插入数据的ID:", task.ID)
	models.Tasks = append(models.Tasks, *task)

	if task.IsEnabled {
		services.ScheduleTask(task)
	}
	return c.JSON(http.StatusOK, task)

	// task.ID = fmt.Sprintf("%d", len(models.Tasks)+1)
	// models.Tasks = append(models.Tasks, *task)
	// models.WriteTasksToJSON()

	// if task.IsEnabled {
	// 	services.ScheduleTask(task)
	// }

	// return c.JSON(http.StatusCreated, task)
}

func UpdateTaskHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	task := new(models.Task)
	if err := models.DB.First(&task, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}
	if err := c.Bind(task); err != nil {
		return err
	}

	models.TaskMutex.Lock()
	defer models.TaskMutex.Unlock()

	for i, t := range models.Tasks {
		if t.ID == uint(id) {
			// 保存旧的 CronEntryID
			oldCronEntryID := t.CronEntryID

			// 更新任务
			task.ID = uint(id)
			task.CronEntryID = oldCronEntryID // 保持原有的 CronEntryID
			models.Tasks[i] = *task

			// 移除旧的定时器
			if oldCronEntryID != 0 {
				models.CronJob.Remove(oldCronEntryID)
				fmt.Printf("Removed old cron entry: %v\n", oldCronEntryID)
			}

			// 如果任务启用，重新调度
			if task.IsEnabled {
				services.ScheduleTask(task)
			} else {
				// 如果任务被禁用，确保 CronEntryID 被重置
				task.CronEntryID = 0
				models.Tasks[i].CronEntryID = 0
			}

			// 无论任务是否启用，都更新数据库
			if err := models.DB.Save(&task).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update task"})
			}

			services.PrintActiveTasks()

			return c.JSON(http.StatusOK, task)
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
}

func DeleteTaskHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var t models.Task
	if err := models.DB.First(&t, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	models.TaskMutex.Lock()
	defer models.TaskMutex.Unlock()

	for i, task := range models.Tasks {
		if task.ID == uint(id) {
			models.CronJob.Remove(task.CronEntryID)
			fmt.Printf("Removed cron entry: %v\n", task.CronEntryID)
			models.Tasks = append(models.Tasks[:i], models.Tasks[i+1:]...)
			if err := models.DB.Delete(&task).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete task"})
			}
			return c.NoContent(http.StatusNoContent)
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
}

func ToggleTaskHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var t models.Task
	if err := models.DB.First(&t, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	models.TaskMutex.Lock()
	defer models.TaskMutex.Unlock()

	for i, task := range models.Tasks {
		if task.ID == uint(id) {
			models.Tasks[i].IsEnabled = !task.IsEnabled
			if models.Tasks[i].IsEnabled {
				services.ScheduleTask(&models.Tasks[i])
			} else {
				if models.Tasks[i].CronEntryID != 0 {
					models.CronJob.Remove(models.Tasks[i].CronEntryID)
					fmt.Printf("Removed cron entry: %v\n", models.Tasks[i].CronEntryID)
					models.Tasks[i].CronEntryID = 0
					services.PrintActiveTasks()
				}
			}
			if err := models.DB.Save(&task).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update task"})
			}
			return c.JSON(http.StatusOK, models.Tasks[i])
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
}

func RunTaskHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	for _, task := range models.Tasks {
		if task.ID == uint(id) {
			go services.ExecuteTask(&task)
			return c.JSON(http.StatusOK, map[string]string{"message": "Task started"})
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
}

// func GetTaskLogsHandler(c echo.Context) error {
// 	id, _ := strconv.Atoi(c.Param("id"))

// 	for _, task := range models.Tasks {
// 		if task.ID == uint(id) {
// 			logFile := filepath.Join(filepath.Join("logs", fmt.Sprintf("task_%d", task.ID)), "output.log")
// 			logs, err := os.ReadFile(logFile)
// 			if err != nil {
// 				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Error reading log file"})
// 			}
// 			return c.String(http.StatusOK, string(logs))
// 		}
// 	}

// 	return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
// }

func GetTaskLogsHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var task *models.Task
	for _, t := range models.Tasks {
		if t.ID == uint(id) {
			task = &t
			break
		}
	}
	if task == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Task not found"})
	}

	logFile := filepath.Join(filepath.Join("logs", fmt.Sprintf("task_%d", task.ID)), "output.log")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	file, err := os.Open(logFile)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: Error opening log file: %s\n\n", err.Error()))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: Error getting file info: %s\n\n", err.Error()))
	}

	byteCount := fileInfo.Size()
	offset := utils.CalculateOffset(file, byteCount, 10000) // 假设有这个函数

	tailFile, err := tail.TailFile(logFile, tail.Config{
		ReOpen:    true,
		Follow:    true,
		Location:  &tail.SeekInfo{Offset: offset, Whence: 2},
		MustExist: false,
		Poll:      true,
	})
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: Failed to tail log file: %s\n\n", err.Error()))
	}
	defer tailFile.Cleanup()

	c.Response().Writer.WriteHeader(http.StatusOK)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-c.Request().Context().Done():
				return
			case line, ok := <-tailFile.Lines:
				if !ok {
					c.String(http.StatusInternalServerError, "data: Tail file closed unexpectedly\n\n")
					return
				}
				if line.Err != nil {
					c.String(http.StatusInternalServerError, fmt.Sprintf("data: Error reading log file: %s\n\n", line.Err.Error()))
					return
				}
				trimmedLine := strings.TrimRight(utils.EnsureUTF8(line.Text), "\r\n")
				if trimmedLine != "" {
					if _, err := fmt.Fprintf(c.Response(), "data: %s\n\n", trimmedLine); err != nil {
						return
					}
					c.Response().Flush()
				}
			}
		}
	}()

	<-c.Request().Context().Done()
	<-done

	return nil
}
