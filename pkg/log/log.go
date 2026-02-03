package log

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sugarLogger *zap.SugaredLogger
var zapLogger *zap.Logger // 新增：导出原始 logger

func Init(level, format, outputpath string) {
	var err error
	var logger *zap.Logger
	var zapConfig zap.Config

	// 根据配置设置日志级别
	LogLevel := zap.NewAtomicLevel()
	if err := LogLevel.UnmarshalText([]byte(level)); err != nil {
		LogLevel.SetLevel(zap.InfoLevel)
		panic(fmt.Errorf("invalid log level: %w", err))
	}

	// 根据配置设置编码格式
	encoding := "json"
	if format == "console" {
		encoding = "console"
	}

	// 配置开发环境
	if format == "console" {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	} else {
		// 配置生产环境
		zapConfig = zap.NewProductionConfig()
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}

	zapConfig.Level = LogLevel
	zapConfig.Encoding = encoding
	zapConfig.OutputPaths = []string{"stdout"}
	if outputpath != "" {
		// 如果指定了文件输出路径，同时输出到文件和 stdout
		// 确保目录存在
		if err := os.MkdirAll(outputpath, 0755); err != nil {
			panic(fmt.Errorf("failed to create log directory: %w", err))
		}
		zapConfig.OutputPaths = append(zapConfig.OutputPaths, outputpath+"/app.log")
	}

	// 构建logger
	if logger, err = zapConfig.Build(); err != nil {
		panic(fmt.Errorf("failed to build logger: %w", err))
	}

	zapLogger = logger // 新增：导出原始 logger
	sugarLogger = logger.Sugar()
}

// wrapper（包装）函数，方便替换底层实现
// Info 记录一条 info 级别的日志
func Info(msg string) {
	sugarLogger.Info(msg)
}

// Infof 使用格式化字符串记录一条 info 级别的日志
func Infof(format string, args ...interface{}) {
	sugarLogger.Infof(format, args...)
}

// Infow 使用键值对记录一条 info 级别的日志
func Infow(msg string, keysAndValues ...interface{}) {
	sugarLogger.Infow(msg, keysAndValues...)
}

// Warnf 使用格式化字符串记录一条 warn 级别的日志
func Warnf(template string, args ...interface{}) {
	sugarLogger.Warnf(template, args...)
}

// Error 记录一条 error 级别的日志，并附带 error 信息
func Error(msg string, err error) {
	sugarLogger.Errorw(msg, "error", err)
}

// Fatal 记录一条 fatal 级别的日志，并附带 error 信息，然后退出程序
func Fatal(msg string, err error) {
	sugarLogger.Fatalw(msg, "error", err)
}

func Fatalf(template string, args ...interface{}) {
	sugarLogger.Fatalf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	sugarLogger.Errorf(template, args...)
}

// Sync 将缓冲区中的任何日志刷新（写入）到底层 Writer。
// 在程序退出前调用它是个好习惯。
func Sync() {
	_ = sugarLogger.Sync()
	_ = zapLogger.Sync()
}

func GetLogger() *zap.Logger {
	return zapLogger
}
