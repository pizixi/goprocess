//go:build windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	goprocess "github.com/pizixi/goprocess/cmd"
	"github.com/pizixi/goprocess/internal/config"
	"github.com/pizixi/goprocess/web"
)

func startWithSystray() {
	// 先启动主进程
	go goprocess.GoprocessMain()

	// 等待主进程启动
	time.Sleep(2 * time.Second)

	// 启动系统托盘
	systray.Run(onReady, onExit)
}

func onReady() {
	// 创建一个简单的16x16像素的ICO图标数据
	iconData := []byte{}

	// 尝试加载自定义图标，如果失败就使用默认图标
	if customIcon, err := web.StaticFS.ReadFile("static/images/favicon.ico"); err == nil && len(customIcon) > 0 {
		iconData = customIcon
	}

	// 设置系统托盘图标和属性
	systray.SetIcon(iconData)
	systray.SetTitle("GoProcess")
	systray.SetTooltip("GoProcess 进程管理工具")

	// 添加菜单项
	mOpen := systray.AddMenuItem("打开主界面", "打开 GoProcess 主界面")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("退出", "退出应用程序")

	// 处理菜单点击事件
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openMainWindow()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	// 执行清理工作
	if goprocess.PS != nil {
		goprocess.PS.StopAllProcesses()
	}
	os.Exit(0)
}

// 打开主窗口
func openMainWindow() {
	// 从config.Conf.Addr 中提取端口号
	addr := config.Conf.Addr
	port := strings.Split(addr, ":")[1]
	url := "http://127.0.0.1:" + port
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
		// 隐藏命令行窗口
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
