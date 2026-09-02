package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	Kafka     KafkaConfig
	MinIO     MinIOConfig
	JWT       JWTConfig
	RTMP      RTMPConfig
	RateLimit RateLimitConfig
	TLS       TLSConfig
	Logging   LoggingConfig
	Claude    ClaudeConfig
	Razorpay  RazorpayConfig
	SMS       SMSConfig
	Push      PushConfig
	Codemagic CodemagicConfig
	WhatsApp  WhatsAppConfig
	Email     EmailConfig
	App       AppConfig
	OTP       OTPConfig
	Google    GoogleConfig
	CORS      CORSConfig
}

// OTPConfig controls phone-OTP behaviour. DevMode bypasses real SMS
// delivery and accepts a fixed code — it MUST be false in production and
// startup refuses to boot otherwise.
type OTPConfig struct {
	DevMode  bool
	DevCode  string
	TTLSec   int
	MaxSends int // per phone, per hour
}

// GoogleConfig lists the OAuth client IDs that Google Sign-In ID tokens are
// allowed to be issued for (web, Android, iOS). Empty disables Google login.
type GoogleConfig struct {
	ClientIDs []string
}

// CORSConfig holds the browser origins allowed to call the API. "*" is only
// honoured outside production.
type CORSConfig struct {
	AllowedOrigins []string
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
	Endpoint        string
	AccessKey       string
	SecretKey       string
	UseSSL          bool
	Bucket          string
	MaterialsBucket string
	DownloadsBucket string
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
	// Tenant-keyed budget, mounted separately on tenant-scoped route
	// groups (see middleware.RateLimitPerTenant) so one tenant's traffic
	// spike can't throttle another tenant sharing an IP/NAT. Aggregates
	// many users per tenant, so defaults well above the per-IP budget.
	TenantRequestsPerMinute int
	TenantBurst             int
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
	Provider    string // "msg91" | "" (none)
	AuthKey     string
	SenderID    string // 6-letter DLT sender ID
	OTPTemplate string // MSG91 OTP template ID
	BaseURL     string // override for tests
	TimeoutSec  int
}

// PushConfig configures FCM HTTP v1 for mobile + web push notifications.
// Leave ServiceAccountJSON and ServiceAccountFile both empty to disable
// push; the notifications service then only records in-app rows.
//
// FCM's legacy server-key API (the one keyed on ServerKey) was sunset by
// Google in 2024 — v1 requires a service-account key instead. Set
// ServiceAccountJSON directly (handy for a Docker/K8s secret env var) or
// ServiceAccountFile to a mounted key file path; JSON wins if both are set.
type PushConfig struct {
	Provider           string // "fcm" | ""
	ServiceAccountJSON string // raw service-account key JSON
	ServiceAccountFile string // path to a service-account key JSON file
	BaseURL            string // default https://fcm.googleapis.com
	TimeoutSec         int
	MaxConcurrency     int // bounded worker pool for per-token v1 sends; default 20
}

// CodemagicConfig configures the per-tenant white-label build pipeline.
// Leave WorkflowID empty to short-circuit dispatch — the build trigger
// endpoint then just queues a row for a human operator to pick up.
type CodemagicConfig struct {
	APIToken      string
	WorkflowID    string
	AppID         string
	BaseURL       string // default https://api.codemagic.io
	TimeoutSec    int
	WebhookSecret string // shared secret for the inbound /webhooks/codemagic
}

// WhatsAppConfig configures broadcast/transactional WhatsApp delivery.
// Gupshup-first because it handles Indian DLT compliance and the sandbox
// is easy. Leave APIKey empty to disable — the broadcast endpoint then
// returns a clear error rather than silently dropping messages.
type WhatsAppConfig struct {
	Provider   string // "gupshup" | ""
	APIKey     string
	Source     string // sender phone (E.164)
	AppName    string // Gupshup app name
	BaseURL    string // default https://api.gupshup.io/sm/api/v1
	TimeoutSec int
}

// EmailConfig configures SMTP for transactional email (purchase receipts,
// onboarding confirmations). Leave Host empty to disable — the service
// logs the would-be send and short-circuits without breaking callers.
//
// Plain SMTP is the lowest common denominator and every managed provider
// (SES, Mailgun, Postmark) speaks it, so swapping is an env-var change.
type EmailConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	FromAddr   string // e.g. "School <noreply@example.com>"
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
			Enabled:                 getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMinute:       getEnvInt("RATE_LIMIT_RPM", 120),
			Burst:                   getEnvInt("RATE_LIMIT_BURST", 30),
			TenantRequestsPerMinute: getEnvInt("RATE_LIMIT_TENANT_RPM", 1200),
			TenantBurst:             getEnvInt("RATE_LIMIT_TENANT_BURST", 300),
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
			Model:     getEnv("CLAUDE_MODEL", "claude-sonnet-4-5"),
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
			Provider:           getEnv("PUSH_PROVIDER", ""),
			ServiceAccountJSON: getEnv("FCM_SERVICE_ACCOUNT_JSON", ""),
			ServiceAccountFile: getEnv("FCM_SERVICE_ACCOUNT_FILE", ""),
			BaseURL:            getEnv("FCM_BASE_URL", "https://fcm.googleapis.com"),
			TimeoutSec:         getEnvInt("FCM_TIMEOUT", 6),
			MaxConcurrency:     getEnvInt("FCM_MAX_CONCURRENCY", 20),
		},
		Codemagic: CodemagicConfig{
			APIToken:      getEnv("CODEMAGIC_API_TOKEN", ""),
			WorkflowID:    getEnv("CODEMAGIC_WORKFLOW_ID", ""),
			AppID:         getEnv("CODEMAGIC_APP_ID", ""),
			BaseURL:       getEnv("CODEMAGIC_BASE_URL", "https://api.codemagic.io"),
			TimeoutSec:    getEnvInt("CODEMAGIC_TIMEOUT", 10),
			WebhookSecret: getEnv("CODEMAGIC_WEBHOOK_SECRET", ""),
		},
		WhatsApp: WhatsAppConfig{
			Provider:   getEnv("WHATSAPP_PROVIDER", ""),
			APIKey:     getEnv("WHATSAPP_API_KEY", ""),
			Source:     getEnv("WHATSAPP_SOURCE", ""),
			AppName:    getEnv("WHATSAPP_APP_NAME", ""),
			BaseURL:    getEnv("WHATSAPP_BASE_URL", "https://api.gupshup.io/sm/api/v1"),
			TimeoutSec: getEnvInt("WHATSAPP_TIMEOUT", 8),
		},
		Email: EmailConfig{
			Host:       getEnv("SMTP_HOST", ""),
			Port:       getEnvInt("SMTP_PORT", 587),
			Username:   getEnv("SMTP_USER", ""),
			Password:   getEnv("SMTP_PASS", ""),
			FromAddr:   getEnv("SMTP_FROM", "School <noreply@example.com>"),
			TimeoutSec: getEnvInt("SMTP_TIMEOUT", 8),
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
		OTP: OTPConfig{
			DevMode:  getEnvBool("OTP_DEV_MODE", false),
			DevCode:  getEnv("OTP_DEV_CODE", "123456"),
			TTLSec:   getEnvInt("OTP_TTL_SECONDS", 300),
			MaxSends: getEnvInt("OTP_MAX_SENDS_PER_HOUR", 5),
		},
		Google: GoogleConfig{
			ClientIDs: splitList(getEnv("GOOGLE_CLIENT_IDS", "")),
		},
		CORS: CORSConfig{
			AllowedOrigins: splitList(getEnv("CORS_ALLOWED_ORIGINS", "*")),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// weakSecrets are placeholder values shipped in .env.example. Booting
// production with any of these is refused.
var weakSecrets = map[string]bool{
	"":                    true,
	"access-secret-key":   true,
	"refresh-secret-key":  true,
	"your-secret-key":     true,
	"your-refresh-secret": true,
	"your-access-secret-key-change-in-production":  true,
	"your-refresh-secret-key-change-in-production": true,
	"changeme": true,
}

func (c *Config) validate() error {
	if c.TLS.Enabled && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		return fmt.Errorf("TLS enabled but cert/key file not provided")
	}

	// Google sign-in, when enabled, always requires the audience allow-list
	// (any env). Without it we'd accept tokens minted for any app.
	if c.Server.Env != "production" {
		return nil
	}

	// ---- production-only hard requirements ----
	var errs []string
	req := func(cond bool, msg string) {
		if cond {
			errs = append(errs, msg)
		}
	}

	req(weakSecrets[c.JWT.AccessSecret] || len(c.JWT.AccessSecret) < 32,
		"JWT_ACCESS_SECRET must be a strong secret (>=32 chars)")
	req(weakSecrets[c.JWT.RefreshSecret] || len(c.JWT.RefreshSecret) < 32,
		"JWT_REFRESH_SECRET must be a strong secret (>=32 chars)")
	req(c.JWT.AccessSecret == c.JWT.RefreshSecret,
		"JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	req(c.Database.Password == "postgres" || c.Database.Password == "app_user_dev_password" || c.Database.Password == "",
		"DB_PASSWORD must not be a default/blank value")
	req(c.Database.SSLMode == "disable" || c.Database.SSLMode == "",
		"DB_SSLMODE must be require/verify-ca/verify-full in production")
	req(c.OTP.DevMode, "OTP_DEV_MODE must be false in production")
	req(c.SMS.Provider == "" || c.SMS.AuthKey == "",
		"an SMS provider must be configured in production (OTP delivery)")
	req(c.Razorpay.KeyID == "" || c.Razorpay.KeySecret == "",
		"RAZORPAY_KEY_ID/RAZORPAY_KEY_SECRET are required in production")
	req(c.Razorpay.WebhookSecret == "",
		"RAZORPAY_WEBHOOK_SECRET is required in production")
	req(c.MinIO.AccessKey == "minioadmin" || c.MinIO.SecretKey == "minioadmin",
		"MinIO credentials must not be the minioadmin defaults in production")
	req(contains(c.CORS.AllowedOrigins, "*"),
		"CORS_ALLOWED_ORIGINS must be an explicit allow-list in production (no '*')")
	req(len(c.Google.ClientIDs) == 0,
		"GOOGLE_CLIENT_IDS is required in production (Google sign-in token audience)")

	if len(errs) > 0 {
		return fmt.Errorf("invalid production config:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
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
