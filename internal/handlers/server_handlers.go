package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/pkg/utils"
)

func GetServerLogsHandler(c echo.Context) error {
	logFile := filepath.Join("logs", "goprocess.log")
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	logReader := utils.NewLogReader(logFile, 10000)
	if err := logReader.Start(); err != nil {
		return fmt.Errorf("启动日志读取器时出错: %w", err)
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
