package models

import (
	"github.com/robfig/cron/v3"
)

type Task struct {
	ID          uint         `json:"ID" gorm:"primaryKey"`
	Name        string       `json:"name"`
	Cron        string       `json:"cron"`
	WorkDir     string       `json:"workDir"`
	Command     string       `json:"command"`
	IsEnabled   bool         `json:"isEnabled"`
	Timeout     int          `json:"timeout"`
	CronEntryID cron.EntryID `json:"-" gorm:"-"`
}
