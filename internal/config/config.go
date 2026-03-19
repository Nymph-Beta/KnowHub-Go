// Package config 负责加载和管理应用程序的配置。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// 全局配置变量，存储从配置文件加载的所有设置。
var Conf Config

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Log           LogConfig           `mapstructure:"log"`
	Database      DatabaseConfig      `mapstructure:"database"`
	JWT           JWTConfig           `mapstructure:"jwt"`
	MinIO         MinIOConfig         `mapstructure:"minio"`
	Kafka         KafkaConfig         `mapstructure:"kafka"`
	Tika          TikaConfig          `mapstructure:"tika"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"`
	Embedding     EmbeddingConfig     `mapstructure:"embedding"`
	LLM           LLMConfig           `mapstructure:"llm"`
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

type MinIOConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl"`
	BucketName      string `mapstructure:"bucket_name"`
}

type KafkaConfig struct {
	Brokers            []string `mapstructure:"brokers"`
	Topic              string   `mapstructure:"topic"`
	GroupID            string   `mapstructure:"group_id"`
	MaxRetry           int      `mapstructure:"max_retry"`
	RetryKeyTTLSeconds int      `mapstructure:"retry_key_ttl_seconds"`
}

type TikaConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type ElasticsearchConfig struct {
	Addresses      []string `mapstructure:"addresses"`
	Username       string   `mapstructure:"username"`
	Password       string   `mapstructure:"password"`
	IndexName      string   `mapstructure:"index_name"`
	VectorDims     int      `mapstructure:"vector_dims"`
	Analyzer       string   `mapstructure:"analyzer"`
	SearchAnalyzer string   `mapstructure:"search_analyzer"`
	RefreshOnWrite bool     `mapstructure:"refresh_on_write"`
}

type EmbeddingConfig struct {
	APIKey         string `mapstructure:"api_key"`
	BaseURL        string `mapstructure:"base_url"`
	Model          string `mapstructure:"model"`
	Dimensions     int    `mapstructure:"dimensions"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type LLMConfig struct {
	Provider                    string              `mapstructure:"provider"`
	APIStyle                    string              `mapstructure:"api_style"`
	APIKey                      string              `mapstructure:"api_key"`
	BaseURL                     string              `mapstructure:"base_url"`
	Model                       string              `mapstructure:"model"`
	TimeoutSeconds              int                 `mapstructure:"timeout_seconds"`
	WebSocketTokenExpireMinutes int                 `mapstructure:"websocket_token_expire_minutes"`
	Generation                  LLMGenerationConfig `mapstructure:"generation"`
	Prompt                      LLMPromptConfig     `mapstructure:"prompt"`
}

type LLMGenerationConfig struct {
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top_p"`
	MaxTokens   int     `mapstructure:"max_tokens"`
}

type LLMPromptConfig struct {
	TemplateFile string `mapstructure:"template_file"`
	Template     string `mapstructure:"-"`
	Rules        string `mapstructure:"rules"`
	RefStart     string `mapstructure:"ref_start"`
	RefEnd       string `mapstructure:"ref_end"`
	NoResultText string `mapstructure:"no_result_text"`
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

	if err := loadPromptTemplate(configPath, &Conf.LLM); err != nil {
		panic(fmt.Errorf("fatal error loading llm prompt template: %w", err))
	}
}

func loadPromptTemplate(configPath string, llmCfg *LLMConfig) error {
	if llmCfg == nil {
		return nil
	}

	templateFile := strings.TrimSpace(llmCfg.Prompt.TemplateFile)
	if templateFile == "" {
		return nil
	}

	if !filepath.IsAbs(templateFile) {
		templateFile = filepath.Join(filepath.Dir(configPath), templateFile)
	}

	content, err := os.ReadFile(templateFile)
	if err != nil {
		return err
	}
	llmCfg.Prompt.Template = string(content)
	return nil
}
