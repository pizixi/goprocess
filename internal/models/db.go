package models

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

const dbPath = "processes.db"

func InitDB() error {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return err
	}
	// 自动迁移
	err = DB.AutoMigrate(&Process{}, &Task{})
	if err != nil {
		return err
	}

	// 初始化 RuntimeProcesses
	RuntimeProcesses = make(map[uint]*RuntimeProcess)
	var processes []Process
	DB.Find(&processes)

	for _, p := range processes {
		rp := &RuntimeProcess{
			Process:    p,
			PID:        0,
			Status:     "stopped",
			ManualStop: false,
		}
		RuntimeProcesses[p.ID] = rp
	}
	// 初始化任务
	DB.Find(&Tasks)

	return nil
}
