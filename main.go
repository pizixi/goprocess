package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	goprocess "github.com/pizixi/goprocess/cmd"
)

var (
	logger service.Logger
)

type program struct{}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	log.Println("Stopping service...")
	// ps.StopAllProcesses() // 直接调用停止所有进程的函数
	if goprocess.PS != nil {
		goprocess.PS.StopAllProcesses()
	}
	return nil
}

func (p *program) run() {
	goprocess.GoprocessMain()
}

func main() {
	curDir, err := os.Getwd()
	if err != nil {
		logger.Error(err)
	}
	svcConfig := &service.Config{
		Name:             "GoProcess",
		DisplayName:      "GoProcess Service",
		Description:      "This is a golang process management service.",
		WorkingDirectory: curDir, // 好像没用
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}
	// 获取当前可执行文件所在目录
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}

	// 设置工作目录
	err = os.Chdir(dir)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		// 带参数时，执行服务控制操作（安装、卸载、启动、停止等）
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}

	// // 不带参数时，以服务方式运行
	// err = s.Run()
	// if err != nil {
	// 	logger.Error(err)
	// }

	// 不带参数时，根据操作系统选择启动方式
	if runtime.GOOS == "windows" {
		// Windows系统使用系统托盘模式启动
		startWithSystray()
	} else {
		// 非Windows系统直接运行主程序
		goprocess.GoprocessMain()
	}
}
