package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App       AppConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	JWT       JWTConfig
	IMAP      IMAPConfig
	SMTP      SMTPConfig
	JMAP      JMAPConfig
	Crypto    CryptoConfig
	WebSocket WebSocketConfig
}

type AppConfig struct {
	Env          string
	Port         string
	AllowOrigins string
	LogLevel     string
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	MigrationsPath  string
}

type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type JWTConfig struct {
	AccessSecret        string
	RefreshSecret       string
	AccessExpiry        time.Duration
	RefreshExpiry       time.Duration
	TempTokenExpiry     time.Duration // Untuk 2FA step-1
	TrustedDeviceExpiry time.Duration
}

type IMAPConfig struct {
	Host     string
	Port     int
	UseTLS   bool
	PoolSize int // Max koneksi IMAP per user
}

type SMTPConfig struct {
	Host   string
	Port   int
	UseTLS bool
}

type JMAPConfig struct {
	BaseURL string
	Enabled bool
}

type CryptoConfig struct {
	TOTPEncryptionKey string // 32 bytes hex string untuk AES-256-GCM
	AppKey            string // General app encryption key
}

type WebSocketConfig struct {
	Enabled  bool
	Interval time.Duration
}

// Load membaca konfigurasi dari environment variables
func Load() (*Config, error) {
	// Load .env jika ada (development)
	_ = godotenv.Load()

	cfg := &Config{
		App: AppConfig{
			Env:          getEnv("APP_ENV", "development"),
			Port:         getEnv("APP_PORT", "8080"),
			AllowOrigins: getEnv("ALLOW_ORIGINS", "http://localhost:3000"),
			LogLevel:     getEnv("LOG_LEVEL", "info"),
		},
		Database: DatabaseConfig{
			DSN: getEnvRequired("DATABASE_URL"),
			// contoh: postgres://user:pass@localhost:5432/webmail?sslmode=disable
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5)) * time.Minute,
			MigrationsPath:  getEnv("MIGRATIONS_PATH", "file://migrations"),
		},
		Redis: RedisConfig{
			Addr:         getEnv("REDIS_ADDR", "localhost:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getEnvInt("REDIS_DB", 0),
			PoolSize:     getEnvInt("REDIS_POOL_SIZE", 20),
			MinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", 5),
			DialTimeout:  time.Duration(getEnvInt("REDIS_DIAL_TIMEOUT_SEC", 5)) * time.Second,
			ReadTimeout:  time.Duration(getEnvInt("REDIS_READ_TIMEOUT_SEC", 3)) * time.Second,
			WriteTimeout: time.Duration(getEnvInt("REDIS_WRITE_TIMEOUT_SEC", 3)) * time.Second,
		},
		JWT: JWTConfig{
			AccessSecret:        getEnvRequired("JWT_ACCESS_SECRET"),
			RefreshSecret:       getEnvRequired("JWT_REFRESH_SECRET"),
			AccessExpiry:        time.Duration(getEnvInt("JWT_ACCESS_EXPIRY_MINUTES", 60)) * time.Minute,
			RefreshExpiry:       time.Duration(getEnvInt("JWT_REFRESH_EXPIRY_DAYS", 7)) * 24 * time.Hour,
			TempTokenExpiry:     5 * time.Minute,
			TrustedDeviceExpiry: 30 * 24 * time.Hour,
		},
		IMAP: IMAPConfig{
			Host:     getEnv("IMAP_HOST", "localhost"),
			Port:     getEnvInt("IMAP_PORT", 993),
			UseTLS:   getEnvBool("IMAP_USE_TLS", true),
			PoolSize: getEnvInt("IMAP_POOL_SIZE", 5),
		},
		SMTP: SMTPConfig{
			Host:   getEnv("SMTP_HOST", "localhost"),
			Port:   getEnvInt("SMTP_PORT", 587),
			UseTLS: getEnvBool("SMTP_USE_TLS", false), // false = STARTTLS
		},
		JMAP: JMAPConfig{
			BaseURL: getEnv("JMAP_BASE_URL", ""),
			Enabled: getEnvBool("JMAP_ENABLED", false),
		},
		Crypto: CryptoConfig{
			TOTPEncryptionKey: getEnvRequired("TOTP_ENCRYPTION_KEY"),
			AppKey:            getEnvRequired("APP_KEY"),
		},
		WebSocket: WebSocketConfig{
			Enabled:  getEnvBool("WEBSOCKET_ENABLED", true),
			Interval: time.Duration(getEnvInt("WEBSOCKET_INTERVAL_SEC", 60)) * time.Second,
		},
	}

	return cfg, nil
}

func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getEnvRequired(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return val
}

func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultValue
}
