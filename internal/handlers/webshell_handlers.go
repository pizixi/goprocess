package handlers

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/pkg/utils"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func WebshellExecuteCommand(c echo.Context) error {
	command := c.FormValue("command")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
		utils.SetSysProcAttr(cmd)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return c.String(500, fmt.Sprintf("错误: %s", err.Error()))
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return c.String(500, fmt.Sprintf("错误: %s", err.Error()))
	}

	if err := cmd.Start(); err != nil {
		return c.String(500, fmt.Sprintf("错误: %s", err.Error()))
	}

	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextPlainCharsetUTF8)
	c.Response().WriteHeader(200)

	var reader io.Reader
	if runtime.GOOS == "windows" {
		reader = transform.NewReader(io.MultiReader(stdout, stderr), simplifiedchinese.GBK.NewDecoder())
	} else {
		reader = io.MultiReader(stdout, stderr)
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Fprintln(c.Response().Writer, scanner.Text())
		c.Response().Flush()
	}

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(c.Response().Writer, "命令执行完成，但有错误: %v", err)
	}

	return nil
}
