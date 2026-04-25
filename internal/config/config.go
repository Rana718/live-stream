package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Server       ServerConfig
	Database     DatabaseConfig
	Redis        RedisConfig
	Kafka        KafkaConfig
	MinIO        MinIOConfig
	JWT          JWTConfig
	RTMP         RTMPConfig
	RateLimit    RateLimitConfig
	TLS          TLSConfig
	Logging      LoggingConfig
	Claude       ClaudeConfig
	Razorpay     RazorpayConfig
	SMS          SMSConfig
	Push         PushConfig
	App          AppConfig
}

type ServerConfig struct {
	Port            string
	Env             string
	ReadTimeoutSec  int
	WriteTimeoutSec int
	IdleTimeoutSec  int
	ShutdownTimeout int
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime int
	MaxConnIdleTime int
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

type MinIOConfig struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	UseSSL           bool
	Bucket           string
	MaterialsBucket  string
	DownloadsBucket  string
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessExpiry  string
	RefreshExpiry string
}

type RTMPConfig struct {
	ServerURL string
	StreamKey string
}

type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
	Burst             int
}

type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

type LoggingConfig struct {
	Level  string // debug, info, warn, error
	Format string // json, text
}

type ClaudeConfig struct {
	APIKey    string
	Model     string
	MaxTokens int
}

type RazorpayConfig struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
}

// SMSConfig configures the SMS provider used for OTP delivery. We support
// MSG91 (Indian DLT-compliant) out of the box. Leave AuthKey empty to
// disable SMS dispatch — the dev OTP flow short-circuits in that case.
type SMSConfig struct {
	Provider     string // "msg91" | "" (none)
	AuthKey      string
	SenderID     string // 6-letter DLT sender ID
	OTPTemplate  string // MSG91 OTP template ID
	BaseURL      string // override for tests
	TimeoutSec   int
}

// PushConfig configures FCM for mobile + web push notifications. Leave
// ServerKey empty to disable push; the notifications service then only
// records in-app rows.
type PushConfig struct {
	Provider   string // "fcm" | ""
	ServerKey  string // FCM legacy server key
	BaseURL    string // default https://fcm.googleapis.com/fcm/send
	TimeoutSec int
}

type AppConfig struct {
	BaseURL       string
	HLSBaseURL    string
	RTMPBaseURL   string
	DefaultLocale string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("SERVER_PORT", "3000"),
			Env:             getEnv("ENV", "development"),
			ReadTimeoutSec:  getEnvInt("SERVER_READ_TIMEOUT", 30),
			WriteTimeoutSec: getEnvInt("SERVER_WRITE_TIMEOUT", 30),
			IdleTimeoutSec:  getEnvInt("SERVER_IDLE_TIMEOUT", 120),
			ShutdownTimeout: getEnvInt("SERVER_SHUTDOWN_TIMEOUT", 30),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", "postgres"),
			DBName:          getEnv("DB_NAME", "live_platform"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxConns:        int32(getEnvInt("DB_MAX_CONNS", 25)),
			MinConns:        int32(getEnvInt("DB_MIN_CONNS", 5)),
			MaxConnLifetime: getEnvInt("DB_MAX_CONN_LIFETIME", 3600),
			MaxConnIdleTime: getEnvInt("DB_MAX_CONN_IDLE", 1800),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Brokers: []string{getEnv("KAFKA_BROKER", "localhost:9092")},
			Topic:   getEnv("KAFKA_TOPIC", "stream-events"),
		},
		MinIO: MinIOConfig{
			Endpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey:       getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey:       getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("MINIO_USE_SSL", false),
			Bucket:          getEnv("MINIO_BUCKET", "recordings"),
			MaterialsBucket: getEnv("MINIO_MATERIALS_BUCKET", "materials"),
			DownloadsBucket: getEnv("MINIO_DOWNLOADS_BUCKET", "downloads"),
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "access-secret-key"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "refresh-secret-key"),
			AccessExpiry:  getEnv("JWT_ACCESS_EXPIRY", "15m"),
			RefreshExpiry: getEnv("JWT_REFRESH_EXPIRY", "7d"),
		},
		RTMP: RTMPConfig{
			ServerURL: getEnv("RTMP_SERVER_URL", "rtmp://localhost:1935/live"),
			StreamKey: getEnv("RTMP_STREAM_KEY", ""),
		},
		RateLimit: RateLimitConfig{
			Enabled:           getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMinute: getEnvInt("RATE_LIMIT_RPM", 120),
			Burst:             getEnvInt("RATE_LIMIT_BURST", 30),
		},
		TLS: TLSConfig{
			Enabled:  getEnvBool("TLS_ENABLED", false),
			CertFile: getEnv("TLS_CERT_FILE", ""),
			KeyFile:  getEnv("TLS_KEY_FILE", ""),
		},
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Claude: ClaudeConfig{
			APIKey:    getEnv("CLAUDE_API_KEY", ""),
			Model:     getEnv("CLAUDE_MODEL", "claude-sonnet-4-6"),
			MaxTokens: getEnvInt("CLAUDE_MAX_TOKENS", 2048),
		},
		SMS: SMSConfig{
			Provider:    getEnv("SMS_PROVIDER", ""),
			AuthKey:     getEnv("SMS_AUTH_KEY", ""),
			SenderID:    getEnv("SMS_SENDER_ID", ""),
			OTPTemplate: getEnv("SMS_OTP_TEMPLATE", ""),
			BaseURL:     getEnv("SMS_BASE_URL", "https://control.msg91.com/api/v5"),
			TimeoutSec:  getEnvInt("SMS_TIMEOUT", 8),
		},
		Push: PushConfig{
			Provider:   getEnv("PUSH_PROVIDER", ""),
			ServerKey:  getEnv("FCM_SERVER_KEY", ""),
			BaseURL:    getEnv("FCM_BASE_URL", "https://fcm.googleapis.com/fcm/send"),
			TimeoutSec: getEnvInt("FCM_TIMEOUT", 6),
		},
		Razorpay: RazorpayConfig{
			KeyID:         getEnv("RAZORPAY_KEY_ID", ""),
			KeySecret:     getEnv("RAZORPAY_KEY_SECRET", ""),
			WebhookSecret: getEnv("RAZORPAY_WEBHOOK_SECRET", ""),
		},
		App: AppConfig{
			BaseURL:       getEnv("APP_BASE_URL", "http://localhost:3000"),
			HLSBaseURL:    getEnv("HLS_BASE_URL", "http://localhost:8080/hls"),
			RTMPBaseURL:   getEnv("RTMP_BASE_URL", "rtmp://localhost:1935/live"),
			DefaultLocale: getEnv("DEFAULT_LOCALE", "en"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Env == "production" {
		if c.JWT.AccessSecret == "access-secret-key" || c.JWT.RefreshSecret == "refresh-secret-key" {
			return fmt.Errorf("production requires strong JWT secrets (set JWT_ACCESS_SECRET and JWT_REFRESH_SECRET)")
		}
		if c.Database.Password == "postgres" {
			return fmt.Errorf("production requires a non-default DB password")
		}
	}
	if c.TLS.Enabled && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		return fmt.Errorf("TLS enabled but cert/key file not provided")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultValue
}
