package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Record struct {
	ID              uint   `json:"id" gorm:"primaryKey"`
	RequestID       string `json:"request_id" gorm:"size:128;index"`
	UserID          int    `json:"user_id" gorm:"index"`
	TokenID         int    `json:"token_id"`
	Point           string `json:"point" gorm:"size:64;index"`
	Attempt         int    `json:"attempt"`
	ChannelID       int    `json:"channel_id"`
	Protocol        string `json:"protocol" gorm:"size:32"`
	Model           string `json:"model" gorm:"size:255"`
	Stream          bool   `json:"stream"`
	Debug           bool   `json:"debug"`
	Status          string `json:"status" gorm:"size:32;index"`
	Action          string `json:"action" gorm:"size:32"`
	Hits            int    `json:"hits"`
	ErrorCode       string `json:"error_code" gorm:"size:64"`
	CreatedAt       int64  `json:"created_at" gorm:"index"`
	UpdatedAt       int64  `json:"updated_at"`
	DurationMs      int64  `json:"duration_ms"`
	InputEvents     int    `json:"input_events"`
	OutputEvents    int    `json:"output_events"`
	InputBytes      int64  `json:"input_bytes"`
	OutputBytes     int64  `json:"output_bytes"`
	Input           string `json:"input" gorm:"size:65536"`
	Output          string `json:"output" gorm:"size:65536"`
	InputTruncated  bool   `json:"input_truncated"`
	OutputTruncated bool   `json:"output_truncated"`
	Terminal        bool   `json:"terminal"`
}

func (Record) TableName() string { return "example_relay_records" }

type Store struct{ DB *gorm.DB }

func Open(v *viper.Viper) (*Store, error) {
	if mode := v.GetString("database.mode"); mode != "independent" && mode != "shared" {
		return nil, errors.New("database.mode 必须为 independent 或 shared")
	}
	var dialector gorm.Dialector
	switch v.GetString("database.driver") {
	case "sqlite":
		path := v.GetString("database.sqlite_path")
		if path == "" {
			return nil, errors.New("database.sqlite_path 必填")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return nil, err
		}
		dialector = sqlite.Open(fmt.Sprintf("%s?_busy_timeout=%d", path, v.GetInt("database.sqlite_busy_timeout")))
	case "mysql":
		if v.GetString("database.dsn") == "" {
			return nil, errors.New("database.dsn 必填")
		}
		dialector = mysql.Open(v.GetString("database.dsn"))
	case "postgres":
		if v.GetString("database.dsn") == "" {
			return nil, errors.New("database.dsn 必填")
		}
		dialector = postgres.Open(v.GetString("database.dsn"))
	default:
		return nil, errors.New("不支持的数据库类型")
	}
	db, err := gorm.Open(dialector, &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if v.GetString("database.driver") == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	}
	if err = db.AutoMigrate(&Record{}); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Start(ctx context.Context, record *Record) error {
	return s.DB.WithContext(ctx).Create(record).Error
}

func (s *Store) Save(ctx context.Context, record *Record) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return s.DB.WithContext(writeCtx).Save(record).Error
}
