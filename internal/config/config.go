// Package config 负责加载和管理应用程序的配置。
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// 全局配置变量，存储从配置文件加载的所有设置。
var Conf Config

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Log      LogConfig      `mapstructure:"log"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
}

// ServerConfig 存储服务器相关的配置。
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type LogConfig struct {
	Level           string `mapstructure:"level"`
	Format          string `mapstructure:"format"`
	OutputPath      string `mapstructure:"output_path"`
	ErrorOutputPath string `mapstructure:"error_output_path"`
	Maxsize         int    `mapstructure:"maxsize"`
	Maxbackups      int    `mapstructure:"maxbackups"`
	Maxage          int    `mapstructure:"maxage"`
	Compress        bool   `mapstructure:"compress"`
	TimeFormat      string `mapstructure:"time_format"`
}

type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql"`
	Redis RedisConfig `mapstructure:"redis"`
}

type MySQLConfig struct {
	DSN string `mapstructure:"dsn"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret                 string `mapstructure:"secret"`
	AccessTokenExpireHours int    `mapstructure:"access_token_expire_hours"`
	RefreshTokenExpireDays int    `mapstructure:"refresh_token_expire_days"`
}

// init 初始化配置加载，从指定的路径读取 YAML 配置文件并解析导入到 Conf 变量中
func Init(configPath string) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	if err := viper.Unmarshal(&Conf); err != nil {
		panic(fmt.Errorf("fatal error unmarshalling config: %w", err))
	}
}
