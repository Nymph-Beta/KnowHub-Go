package main

import (
	"errors"
	"fmt"
	"pai_smart_go_v2/internal/config"
	"pai_smart_go_v2/internal/middleware"
	"pai_smart_go_v2/pkg/database"
	"pai_smart_go_v2/pkg/log"

	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"context"

	"github.com/gin-gonic/gin"
)

func main() {
	config.Init("configs/config.yaml")
	fmt.Println(config.Conf)
	cfg := config.Conf

	log.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.OutputPath)
	defer log.Sync()

	log.Info("Server started")

	// GormDemo()

	database.InitMySQL(cfg.Database.MySQL.DSN)
	if err := database.RunMigrate(); err != nil {
		log.Fatal("Failed to run migrations", err)
		return
	}
	// TestLog()

	// router := gin.Default()
	// router.GET("/ping", func(c *gin.Context) {
	// 	c.JSON(200, gin.H{"message": "pong"})
	// })
	// router.Run(":" + cfg.Server.Port)

	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(middleware.RequestLogger(), gin.Recovery())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	r.POST("/echo", func(c *gin.Context) {
		var body map[string]interface{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "success", "data": body})
	})

	// r.Run(":" + cfg.Server.Port)

	// 启动 HTTP 服务器并实现优雅停机
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Infof("服务启动于 %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务监听失败: %s\n", err)
		}
	}()

	// 等待中断信号以实现优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("接收到停机信号，正在关闭服务...")

	// 设置一个5秒的超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("HTTP 服务器关闭失败: %v", err)
	}

	log.Info("服务已优雅关闭")

}

func TestLog() {
	log.Info("测试日志输出")
	log.Infof("用户: %s, 年龄: %d", "John", 30)
	log.Infow("用户", "name", "John", "age", 30)

	log.Warnf("配置项 %s 已废弃，请使用 %s", "old_key", "new_key")
	log.Error("测试错误", errors.New("测试错误"))
	log.Errorf("测试错误 with format: %s", "test error")

	// log.Fatal("测试致命错误", errors.New("测试致命错误"))
	// log.Fatalf("测试致命错误 with format: %s", "test fatal")
}
