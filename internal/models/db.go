package models

import (
	"context"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

const dbPath = "processes.db"

type Database interface {
	Find(ctx context.Context, dest interface{}, conds ...interface{}) error
	Create(ctx context.Context, value interface{}) error
	Save(ctx context.Context, value interface{}) error
	Delete(ctx context.Context, value interface{}) error
	First(ctx context.Context, dest interface{}, conds ...interface{}) error
}

type GormDatabase struct {
	db *gorm.DB
}

func NewGormDatabase(dbPath string) (*GormDatabase, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.AutoMigrate(&Process{}, &Task{}); err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}

	return &GormDatabase{db: db}, nil
}

func (g *GormDatabase) Find(ctx context.Context, dest interface{}, conds ...interface{}) error {
	return g.db.WithContext(ctx).Find(dest, conds...).Error
}

func (g *GormDatabase) Create(ctx context.Context, value interface{}) error {
	return g.db.WithContext(ctx).Create(value).Error
}

func (g *GormDatabase) Save(ctx context.Context, value interface{}) error {
	return g.db.WithContext(ctx).Save(value).Error
}

func (g *GormDatabase) Delete(ctx context.Context, value interface{}) error {
	return g.db.WithContext(ctx).Delete(value).Error
}

func (g *GormDatabase) First(ctx context.Context, dest interface{}, conds ...interface{}) error {
	return g.db.WithContext(ctx).First(dest, conds...).Error
}
func InitDB() (*gorm.DB, error) {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	// 自动迁移
	err = DB.AutoMigrate(&Process{}, &Task{})
	if err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}

	// // 初始化任务
	// DB.Find(&Tasks)

	return DB, nil
}
