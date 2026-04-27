package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv   string
	HTTPPort string

	DB DBConfig

	JWTSecret     string
	JWTTTLHours   int
	AdminEmail    string
	AdminPassword string
}

type DBConfig struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifeMins int

	ConnectRetries   int
	ConnectRetryWait int
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:        getEnv("APP_ENV", "development"),
		HTTPPort:      getEnv("PORT", getEnv("HTTP_PORT", "8080")),
		DB:            loadDBConfig(),
		JWTSecret:     getEnv("JWT_SECRET", ""),
		JWTTTLHours:   getEnvInt("JWT_TTL_HOURS", 24),
		AdminEmail:    getEnv("ADMIN_EMAIL", "admin@example.com"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "Admin123!"),
	}

	if cfg.JWTSecret == "" {
		slog.Warn("JWT_SECRET is empty; using insecure default. Set it in production.")
		cfg.JWTSecret = "insecure-dev-secret-change-me"
	}
	return cfg
}

func getEnv(k, d string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return d
}

func getEnvInt(k string, d int) int {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

func loadDBConfig() DBConfig {
	driver := strings.ToLower(getEnv("DB_DRIVER", "postgres"))
	dsn := getEnv("DB_DSN", "")

	if dsn == "" {
		switch driver {
		case "postgres":
			dsn = buildPostgresDSN()
		case "sqlite":
			dsn = "data.db"
		}
	}

	return DBConfig{
		Driver:           driver,
		DSN:              dsn,
		MaxOpenConns:     getEnvInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:     getEnvInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifeMins:  getEnvInt("DB_CONN_MAX_LIFE_MINUTES", 30),
		ConnectRetries:   getEnvInt("DB_CONNECT_RETRIES", 10),
		ConnectRetryWait: getEnvInt("DB_CONNECT_RETRY_WAIT_SECONDS", 2),
	}
}

func buildPostgresDSN() string {
	host := getEnv("PG_HOST", "localhost")
	port := getEnv("PG_PORT", "5432")
	user := getEnv("PG_USER", "postgres")
	pass := getEnv("PG_PASSWORD", "postgres")
	name := getEnv("PG_DB", "inventory")
	ssl := getEnv("PG_SSLMODE", "disable")
	tz := getEnv("PG_TIMEZONE", "UTC")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		host, port, user, pass, name, ssl, tz)
}
