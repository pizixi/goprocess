package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/nxadm/tail"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/internal/websocket"
	"github.com/pizixi/goprocess/pkg/utils"
)

func ListProcessesHandler(c echo.Context) error {
	var rps []models.RuntimeProcess
	for _, rp := range models.RuntimeProcesses {
		rps = append(rps, *rp)
	}
	sort.Slice(rps, func(i, j int) bool {
		return rps[i].ID < rps[j].ID
	})
	response := map[string]interface{}{
		"code":  0,
		"msg":   "success",
		"count": len(rps),
		"data":  rps,
	}
	return c.JSON(http.StatusOK, response)
	// return c.JSON(http.StatusOK, rps)
}

func GetProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	return c.JSON(http.StatusOK, rp)
}

func CreateProcessHandler(c echo.Context) error {
	var p models.Process
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if p.RetryCount == 0 {
		p.RetryCount = 3
	}

	result := models.DB.Create(&p)
	if result.Error != nil {
		log.Println(result.Error)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create process"})
	}
	// 获取插入数据的ID
	log.Println("插入数据的ID:", p.ID)
	rp := &models.RuntimeProcess{
		Process:    p,
		PID:        0,
		Status:     "stopped",
		ManualStop: false,
	}
	models.RuntimeProcesses[p.ID] = rp
	return c.JSON(http.StatusOK, rp)
	// p.ID = uint(len(models.RuntimeProcesses) + 2)
	// rp := &models.RuntimeProcess{
	// 	Process:    p,
	// 	PID:        0,
	// 	Status:     "stopped",
	// 	ManualStop: false,
	// }
	// models.RuntimeProcesses[p.ID] = rp
	// if err := models.WriteProcessesToJSON(); err != nil {
	// 	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save process"})
	// }
	// return c.JSON(http.StatusOK, rp)
}

func UpdateProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	if err := c.Bind(&rp.Process); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	// 获取更新数据
	if err := models.DB.Save(&rp.Process).Error; err != nil {
		log.Println(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update process"})
	}
	// if err := models.WriteProcessesToJSON(); err != nil {
	// 	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update process"})
	// }
	return c.JSON(http.StatusOK, rp)
}

func DeleteProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "running" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Process is still running. Please stop it first."})
	}

	delete(models.RuntimeProcesses, uint(id))
	// 删除数据
	if err := models.DB.Delete(&rp.Process).Error; err != nil {
		log.Println(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete process"})
	}
	// if err := models.WriteProcessesToJSON(); err != nil {
	// 	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete process"})
	// }
	return c.JSON(http.StatusOK, map[string]string{"message": "Process deleted"})
}

func StartProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status != "stopped" && rp.Status != "error" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Process %d is %s", rp.ID, rp.Status)})
	}

	rp.ManualStop = false
	go services.StartProcessById(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "starting", "message": "Process is being started"})
}

func StopProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "stopped" {
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "message": "Process already stopped"})
	}

	rp.Status = "stopping"
	rp.ManualStop = true
	websocket.BroadcastStatus(*rp)

	go services.StopProcessByID(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "stopping", "message": "Process is being stopped"})
}

func LogStreamHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := models.RuntimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	file, err := os.Open(rp.LogFile)
	if err != nil {
		log.Printf("无法打开文件: %v\n", err)
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("获取文件信息失败: %v\n", err)
		return err
	}
	byteCount := fileInfo.Size()
	log.Printf("文件 '%s' 的字节数: %d\n", rp.LogFile, byteCount)

	offset := utils.CalculateOffset(file, byteCount, 10000)
	log.Println("offset:", offset)

	if rp.LogFile == "" {
		return c.String(http.StatusInternalServerError, "data: Log file not specified\n\n")
	}

	tailFile, err := tail.TailFile(rp.LogFile, tail.Config{
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
			case msg, ok := <-tailFile.Lines:
				if !ok {
					c.String(http.StatusInternalServerError, "data: Tail file closed unexpectedly\n\n")
					return
				}
				if msg.Err != nil {
					c.String(http.StatusInternalServerError, fmt.Sprintf("data: Error reading log file: %s\n\n", msg.Err.Error()))
					return
				}
				line := strings.TrimRight(utils.EnsureUTF8(msg.Text), "\r\n")
				line = strings.TrimRight(utils.EnsureUTF8(line), "\n")
				if line != "" {
					c.String(http.StatusOK, fmt.Sprintf("data: %s\n\n", line))
					c.Response().Flush()
				}
			}
		}
	}()

	<-c.Request().Context().Done()
	<-done
	return nil
}
