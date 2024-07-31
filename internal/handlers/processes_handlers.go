package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/internal/websocket"
	"github.com/pizixi/goprocess/pkg/utils"
)

type ProcessHandler struct {
	PM *models.ProcessManager
	PS *services.ProcessService
}

func NewHandler(pm *models.ProcessManager, ps *services.ProcessService) *ProcessHandler {
	return &ProcessHandler{PM: pm, PS: ps}
}

func (h *ProcessHandler) ListProcessesHandler(c echo.Context) error {
	rps := h.PM.GetAllProcesses()
	// 根据id排序rps
	models.SortRuntimeProcesses(rps)
	response := map[string]interface{}{
		"code":  0,
		"msg":   "success",
		"count": len(rps),
		"data":  rps,
	}
	return c.JSON(http.StatusOK, response)
}

func (h *ProcessHandler) GetProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	return c.JSON(http.StatusOK, rp)
}

func (h *ProcessHandler) CreateProcessHandler(c echo.Context) error {
	ctx := c.Request().Context()
	var p models.Process
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if p.RetryCount == 0 {
		p.RetryCount = 3
	}

	rp, err := h.PM.AddProcess(ctx, &p)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create process"})
	}
	return c.JSON(http.StatusOK, rp)
}

func (h *ProcessHandler) UpdateProcessHandler(c echo.Context) error {
	ctx := c.Request().Context()
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	if err := c.Bind(&rp.Process); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := h.PM.UpdateProcess(ctx, &rp.Process); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update process"})
	}
	return c.JSON(http.StatusOK, rp)
}

func (h *ProcessHandler) DeleteProcessHandler(c echo.Context) error {
	ctx := c.Request().Context()
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "running" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Process is still running. Please stop it first."})
	}

	if err := h.PM.DeleteProcess(ctx, uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete process"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Process deleted"})
}

func (h *ProcessHandler) StartProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status != "stopped" && rp.Status != "error" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Process %d is %s", rp.ID, rp.Status)})
	}

	h.PM.SetManualStop(uint(id), false)
	go h.PS.StartProcessById(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "starting", "message": "Process is being started"})
}

func (h *ProcessHandler) StopProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "stopped" {
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "message": "Process already stopped"})
	}

	h.PM.UpdateProcessStatus(uint(id), "stopping", rp.PID)
	h.PM.SetManualStop(uint(id), true)
	websocket.BroadcastStatus(*rp)

	go h.PS.StopProcessByID(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "stopping", "message": "Process is being stopped"})
}
func (h *ProcessHandler) GetProcesseLogsHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := h.PM.GetProcess(uint(id))
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.LogFile == "" {
		return c.String(http.StatusInternalServerError, "data: Log file not specified\n\n")
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	logReader := utils.NewLogReader(rp.LogFile, 10000)
	if err := logReader.Start(); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: Failed to start log reader: %s\n\n", err.Error()))
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
