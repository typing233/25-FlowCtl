package config

import (
	"os"
	"time"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Auth       AuthConfig
	Scheduler  SchedulerConfig
	Encryption EncryptionConfig
}

type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
}

type AuthConfig struct {
	JWTSecret       string
	JWTExpiry       time.Duration
	RefreshExpiry   time.Duration
	OIDCIssuerURL   string
	OIDCClientID    string
	OIDCClientSecret string
	SAMLCertPath    string
	SAMLKeyPath     string
	SAMLEntityID    string
	SAMLMetadataURL string
}

type SchedulerConfig struct {
	Enabled           bool
	NodeID            string
	HeartbeatInterval time.Duration
	TakeoverThreshold time.Duration
	PollInterval      time.Duration
}

type EncryptionConfig struct {
	MasterKey string
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://flowctl:flowctl@localhost:5432/flowctl?sslmode=disable"),
			MaxConns:        getEnvInt("DB_MAX_CONNS", 25),
			MinConns:        getEnvInt("DB_MIN_CONNS", 5),
			MaxConnLifetime: 30 * time.Minute,
		},
		Auth: AuthConfig{
			JWTSecret:        getEnv("JWT_SECRET", "dev-secret-change-in-production"),
			JWTExpiry:        15 * time.Minute,
			RefreshExpiry:    7 * 24 * time.Hour,
			OIDCIssuerURL:    getEnv("OIDC_ISSUER_URL", ""),
			OIDCClientID:     getEnv("OIDC_CLIENT_ID", ""),
			OIDCClientSecret: getEnv("OIDC_CLIENT_SECRET", ""),
			SAMLCertPath:     getEnv("SAML_CERT_PATH", ""),
			SAMLKeyPath:      getEnv("SAML_KEY_PATH", ""),
			SAMLEntityID:     getEnv("SAML_ENTITY_ID", ""),
			SAMLMetadataURL:  getEnv("SAML_METADATA_URL", ""),
		},
		Scheduler: SchedulerConfig{
			Enabled:           getEnv("SCHEDULER_ENABLED", "true") == "true",
			NodeID:            getEnv("SCHEDULER_NODE_ID", hostname()),
			HeartbeatInterval: 10 * time.Second,
			TakeoverThreshold: 60 * time.Second,
			PollInterval:      5 * time.Second,
		},
		Encryption: EncryptionConfig{
			MasterKey: getEnv("ENCRYPTION_MASTER_KEY", ""),
		},
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "flowctl-node-1"
	}
	return h
}
