// Package database 提供 MySQL 连接与 GORM 实例的初始化。
package database

import (
	"pai_smart_go_v2/internal/model"
	"pai_smart_go_v2/pkg/log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DB 全局 GORM 数据库实例，在 InitMySQL 成功后可在业务层通过 database.DB 进行 CRUD 等操作。
var DB *gorm.DB

// InitMySQL 根据 DSN 连接 MySQL 并初始化全局 DB。
// 会配置连接池（最大空闲连接数、最大打开连接数、连接最大存活时间），失败时调用 log.Fatal 退出进程。
func InitMySQL(dsn string) {
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to MySQL", err)
	}
	log.Info("Connected to MySQL")

	// 获取底层 *sql.DB 以配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get SQL DB", err)
	}
	sqlDB.SetMaxIdleConns(10)           // 最大空闲连接数
	sqlDB.SetMaxOpenConns(100)          // 最大打开连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大存活时间，超时连接会被回收

	log.Info("MySQL initialized successfully")
}

func RunMigrate() error {
	log.Info("Running migrations...")

	if err := DB.AutoMigrate(
		&model.User{},
		// 后续阶段会继续添加：
		&model.OrganizationTag{}, // 阶段 5
		// &model.Upload{},          // 阶段 6-7
		// &model.ChunkInfo{},       // 阶段 7
		// &model.DocumentVector{},  // 阶段 10
	); err != nil {
		log.Errorf("Failed to run migrations: %v", err)
		return err
	}

	log.Info("Migrations completed successfully")
	return nil
}
