package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/pkg/utils"
)

type TaskHandler struct {
	taskManager *models.TaskManager
	taskService *services.TaskService
}

func NewTaskHandler(tm *models.TaskManager, ts *services.TaskService) *TaskHandler {
	return &TaskHandler{
		taskManager: tm,
		taskService: ts,
	}
}

func (h *TaskHandler) ListTasksHandler(c echo.Context) error {
	tasks := h.taskManager.GetAllTasks()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"code":  0,
		"msg":   "success",
		"count": len(tasks),
		"data":  tasks,
	})
}

func (h *TaskHandler) CreateTaskHandler(c echo.Context) error {
	ctx := c.Request().Context()
	task := new(models.Task)
	if err := c.Bind(task); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的任务数据"})
	}

	task.IsEnabled = false // 创建时默认不启用
	if err := h.taskManager.AddTask(ctx, task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) UpdateTaskHandler(c echo.Context) error {
	ctx := c.Request().Context()
	id, _ := strconv.Atoi(c.Param("id"))
	task, exists := h.taskManager.GetTask(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "任务未找到"})
	}

	if task.IsEnabled {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "不能编辑正在运行的任务"})
	}

	if err := c.Bind(task); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的任务数据"})
	}

	if err := h.taskManager.UpdateTask(ctx, task); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, task)
}

func (h *TaskHandler) DeleteTaskHandler(c echo.Context) error {
	ctx := c.Request().Context()
	id, _ := strconv.Atoi(c.Param("id"))

	if err := h.taskManager.DeleteTask(ctx, uint(id)); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "任务未找到"})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *TaskHandler) RunTaskHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	task, exists := h.taskManager.GetTask(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "任务未找到"})
	}

	go services.ExecuteTask(task)
	return c.JSON(http.StatusOK, map[string]string{"message": "任务已启动"})
}

func (h *TaskHandler) GetTaskLogsHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	task, exists := h.taskManager.GetTask(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "任务未找到"})
	}

	logFile := filepath.Join(filepath.Join("logs", fmt.Sprintf("task_%d", task.ID)), "output.log")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	logReader := utils.NewLogReader(logFile, 10000)
	if err := logReader.Start(); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: 启动日志读取器时出错: %s\n\n", err.Error()))
	}
	defer logReader.Stop()

	c.Response().WriteHeader(http.StatusOK)

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case line, ok := <-logReader.Lines:
			if !ok {
				return nil
			}
			if _, err := fmt.Fprintf(c.Response().Writer, "data: %s\n\n", line); err != nil {
				return err
			}
			c.Response().Flush()
		}
	}
}

func (h *TaskHandler) ToggleTaskStatusHandler(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的任务ID"})
	}

	// 创建一个结构体来接收 JSON 数据
	type ToggleRequest struct {
		IsEnabled bool `json:"isEnabled"`
	}

	var req ToggleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "无效的请求数据"})
	}

	task, exists := h.taskManager.GetTask(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "任务未找到"})
	}

	// 根据新状态调度或取消调度任务
	if req.IsEnabled {
		if err := h.taskService.ScheduleTask(task); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "调度任务失败:" + err.Error()})
		}
	} else {
		h.taskService.UnscheduleTask(task)
	}
	// 更新任务状态
	if err := h.taskManager.ToggleTaskStatus(ctx, uint(id), req.IsEnabled); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "切换任务状态失败"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "任务状态已更新"})
}
