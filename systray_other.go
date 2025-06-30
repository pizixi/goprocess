//go:build !windows

package main

import (
	goprocess "github.com/pizixi/goprocess/cmd"
)

// 非Windows系统的空实现
func startWithSystray() {
	// 非Windows系统直接运行主程序，不使用系统托盘
	goprocess.GoprocessMain()
}
