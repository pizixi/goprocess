//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/getlantern/systray"
	goprocess "github.com/pizixi/goprocess/cmd"
	"github.com/pizixi/goprocess/internal/config"
	"github.com/pizixi/goprocess/web"
)

func startWithSystray() {
	// 添加日志记录
	log.Printf("开始启动系统托盘模式")

	// 使用defer来捕获panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("系统托盘启动发生panic: %v", r)
			// 获取堆栈信息
			buf := make([]byte, 1024*64)
			n := runtime.Stack(buf, false)
			log.Printf("堆栈信息:\n%s", string(buf[:n]))
		}
	}()

	// 先启动主进程
	log.Printf("在goroutine中启动主进程")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("主进程goroutine发生panic: %v", r)
				buf := make([]byte, 1024*64)
				n := runtime.Stack(buf, false)
				log.Printf("主进程堆栈信息:\n%s", string(buf[:n]))
			}
		}()
		goprocess.GoprocessMain()
	}()

	// // 等待主进程启动
	// time.Sleep(2 * time.Second)

	// 启动系统托盘
	log.Printf("启动系统托盘")
	systray.Run(onReady, onExit)
	log.Printf("系统托盘已退出")
}

func onReady() {
	log.Printf("系统托盘onReady函数被调用")

	// 创建一个简单的16x16像素的ICO图标数据
	iconData := []byte{}

	// 尝试加载自定义图标，如果失败就使用默认图标
	if customIcon, err := web.StaticFS.ReadFile("static/images/favicon.ico"); err == nil && len(customIcon) > 0 {
		iconData = customIcon
		log.Printf("成功加载自定义图标，大小: %d bytes", len(iconData))
	} else {
		log.Printf("加载自定义图标失败: %v，使用默认图标", err)
	}

	// 设置系统托盘图标和属性
	systray.SetIcon(iconData)
	systray.SetTitle("GoProcess")
	systray.SetTooltip("GPM®") // 托盘中显示的名称
	log.Printf("系统托盘图标和属性设置完成")

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
	log.Printf("系统托盘onExit函数被调用")

	// 执行清理工作
	if goprocess.PS != nil {
		log.Printf("停止所有进程...")
		goprocess.PS.StopAllProcesses()
		log.Printf("所有进程已停止")
	} else {
		log.Printf("警告: goprocess.PS 为 nil，无法停止进程")
	}

	log.Printf("程序即将退出")
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
