package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kardianos/service"
	"github.com/natefinch/lumberjack"
	goprocess "github.com/pizixi/goprocess/cmd"
)

var (
	logger     service.Logger
	fileLogger *log.Logger
)

type program struct{}

func (p *program) Start(s service.Service) error {
	if fileLogger != nil {
		fileLogger.Printf("服务Start方法被调用")
	}
	go p.run()
	if fileLogger != nil {
		fileLogger.Printf("服务Start方法完成，主程序已在goroutine中启动")
	}
	return nil
}

func (p *program) Stop(s service.Service) error {
	if fileLogger != nil {
		fileLogger.Printf("服务Stop方法被调用")
	}
	log.Println("Stopping service...")
	// ps.StopAllProcesses() // 直接调用停止所有进程的函数
	if goprocess.PS != nil {
		if fileLogger != nil {
			fileLogger.Printf("停止所有进程...")
		}
		goprocess.PS.StopAllProcesses()
		if fileLogger != nil {
			fileLogger.Printf("所有进程已停止")
		}
	} else {
		if fileLogger != nil {
			fileLogger.Printf("警告: goprocess.PS 为 nil，无法停止进程")
		}
	}
	if fileLogger != nil {
		fileLogger.Printf("服务Stop方法完成")
	}
	return nil
}

func (p *program) run() {
	if fileLogger != nil {
		fileLogger.Printf("程序主运行函数开始执行")
	}

	// 使用defer来捕获panic
	defer func() {
		if r := recover(); r != nil {
			if fileLogger != nil {
				fileLogger.Printf("程序发生panic: %v", r)
				// 获取堆栈信息
				buf := make([]byte, 1024*64)
				n := runtime.Stack(buf, false)
				fileLogger.Printf("堆栈信息:\n%s", string(buf[:n]))
			}
		}
		if fileLogger != nil {
			fileLogger.Printf("程序主运行函数结束")
		}
	}()

	goprocess.GoprocessMain()
}

// 初始化文件日志
func initFileLogger() {
	// 确保logs目录存在
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Printf("创建logs目录失败: %v", err)
	}

	// 创建文件日志记录器
	logFile := &lumberjack.Logger{
		Filename:   filepath.Join("logs", "service.log"),
		MaxSize:    100, // MB
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
	}

	fileLogger = log.New(logFile, "", log.LstdFlags|log.Lshortfile)

	// 同时输出到文件和控制台
	log.SetOutput(logFile)

	fileLogger.Printf("=== GoProcess Service Starting ===")
	fileLogger.Printf("时间: %s", time.Now().Format("2006-01-02 15:04:05"))
	fileLogger.Printf("Go版本: %s", runtime.Version())
	fileLogger.Printf("操作系统: %s", runtime.GOOS)
	fileLogger.Printf("架构: %s", runtime.GOARCH)
}

func main() {
	// 首先初始化文件日志
	initFileLogger()

	curDir, err := os.Getwd()
	if err != nil {
		fileLogger.Printf("获取当前目录失败: %v", err)
		if logger != nil {
			logger.Error(err)
		}
	}
	fileLogger.Printf("当前工作目录: %s", curDir)
	svcConfig := &service.Config{
		Name:             "GoProcess",
		DisplayName:      "GoProcess Service",
		Description:      "This is a golang process management service.",
		WorkingDirectory: curDir, // 好像没用
	}

	fileLogger.Printf("创建服务配置: %+v", svcConfig)

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fileLogger.Printf("创建服务失败: %v", err)
		log.Fatal(err)
	}
	fileLogger.Printf("服务创建成功")

	logger, err = s.Logger(nil)
	if err != nil {
		fileLogger.Printf("创建服务日志记录器失败: %v", err)
		log.Fatal(err)
	}
	fileLogger.Printf("服务日志记录器创建成功")
	// 获取当前可执行文件所在目录
	fileLogger.Printf("可执行文件路径: %s", os.Args[0])
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		fileLogger.Printf("获取可执行文件目录失败: %v", err)
		log.Fatal(err)
	}
	fileLogger.Printf("可执行文件目录: %s", dir)

	// 设置工作目录
	err = os.Chdir(dir)
	if err != nil {
		fileLogger.Printf("设置工作目录失败: %v", err)
		log.Fatal(err)
	}
	fileLogger.Printf("工作目录设置成功: %s", dir)
	fileLogger.Printf("命令行参数: %v", os.Args)

	if len(os.Args) > 1 {
		// 带参数时，执行服务控制操作（安装、卸载、启动、停止等）
		action := os.Args[1]
		fileLogger.Printf("执行服务控制操作: %s", action)
		err = service.Control(s, action)
		if err != nil {
			fileLogger.Printf("服务控制操作失败: %v", err)
			fileLogger.Printf("有效的操作: %q", service.ControlAction)
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		fileLogger.Printf("服务控制操作成功: %s", action)
		return
	}

	// 不带参数时，根据运行环境选择启动方式
	isInteractive := service.Interactive()
	fileLogger.Printf("检查运行模式 - Interactive: %v", isInteractive)

	if isInteractive {
		// 交互模式（用户直接运行）
		fileLogger.Printf("运行在交互模式")
		if runtime.GOOS == "windows" {
			// Windows系统使用系统托盘模式启动
			fileLogger.Printf("Windows系统，启动系统托盘模式")
			startWithSystray()
		} else {
			// 非Windows系统直接运行主程序
			fileLogger.Printf("非Windows系统，直接运行主程序")
			goprocess.GoprocessMain()
		}
	} else {
		// 服务模式运行
		fileLogger.Printf("运行在服务模式")
		fileLogger.Printf("开始启动服务...")
		err = s.Run()
		if err != nil {
			fileLogger.Printf("服务运行失败: %v", err)
			logger.Error(err)
		} else {
			fileLogger.Printf("服务正常退出")
		}
	}

	fileLogger.Printf("=== GoProcess Service Exiting ===")
}
