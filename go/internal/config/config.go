package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          int
	Host          string
	DefaultModel  string
	FallbackModel string
	Cache         CacheConfig
	LogLevel      string
	Debug         bool
	CodeBuddy     CodeBuddyConfig
}

type CacheConfig struct {
	Enabled bool
	TTL     time.Duration
	MaxSize int
}

type CodeBuddyConfig struct {
	APIKey      string
	Environment string
}

func Load() *Config {
	return &Config{
		Port:          getEnvInt("PORT", 3000),
		Host:          getEnv("HOST", "0.0.0.0"),
		DefaultModel:  getEnv("DEFAULT_MODEL", "deepseek-v3.1"),
		FallbackModel: getEnv("FALLBACK_MODEL", "deepseek-v3.1"),
		Cache: CacheConfig{
			Enabled: getEnvBool("CACHE_ENABLED", true),
			TTL:     time.Duration(getEnvInt("CACHE_TTL_MS", 300000)) * time.Millisecond,
			MaxSize: getEnvInt("CACHE_MAX_SIZE", 100),
		},
		LogLevel: getEnv("LOG_LEVEL", "info"),
		Debug:    getEnv("LOG_LEVEL", "info") == "debug" || getEnv("NODE_ENV", "production") == "development",
		CodeBuddy: CodeBuddyConfig{
			APIKey:      getEnv("CODEBUDDY_API_KEY", ""),
			Environment: getEnv("CODEBUDDY_INTERNET_ENVIRONMENT", ""),
		},
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1"
	}
	return fallback
}
