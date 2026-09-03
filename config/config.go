package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port string

	// Database PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Email
	EmailHost     string
	EmailPort     int
	EmailUsername string
	EmailPassword string
	EmailFrom     string

	// Session
	SessionSecret string
	SessionExpiry time.Duration
	SessionSecure bool

	// App
	AppName      string
	Debug        bool
	JWTSecret    string
	BaseURL      string
	BasePath     string
	GoogleAPIKey string
}

var AppBasePath = "/Ticketing"

func Path(p string) string {
	if p == "" {
		return AppBasePath
	}
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p
	}
	// Jika sudah ada base path, jangan duplikasi
	if strings.HasPrefix(p, AppBasePath) {
		return p
	}
	return AppBasePath + p
}

func LoadConfig() *Config {
	// Attempt to load .env, but don't fail if it doesn't exist
	_ = godotenv.Load()

	basePath := getEnv("BASE_PATH", "/Ticketing")
	AppBasePath = basePath
	cfg := &Config{
		Port:          getEnv("PORT", "3003"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnvInt("DB_PORT", 5432),
		DBUser:        getEnv("DB_USER", ""),
		DBPassword:    getEnv("DB_PASSWORD", ""),
		DBName:        getEnv("DB_NAME", "ticketing_db"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		EmailHost:     getEnv("EMAIL_HOST", ""),
		EmailPort:     getEnvInt("EMAIL_PORT", 465),
		EmailUsername: getEnv("EMAIL_USER", ""),
		EmailPassword: getEnv("EMAIL_PASSWORD", ""),
		EmailFrom:     getEnv("EMAIL_FROM", ""),
		SessionSecret: getEnv("SESSION_SECRET", ""),
		SessionExpiry: 24 * time.Hour,
		SessionSecure: getEnv("SESSION_SECURE", "true") == "true",
		AppName:       "Ticketing System",
		Debug:         getEnv("DEBUG", "false") == "true",
		JWTSecret:     getEnv("JWT_SECRET", ""),
		BaseURL:       getEnv("BASE_URL", "https://localhost:3000"),
		BasePath:      basePath,
		GoogleAPIKey:  getEnv("GEMINI_API_KEY", ""),
	}

	// [Security] Validasi konfigurasi wajib
	missing := []string{}
	if cfg.DBUser == "" {
		missing = append(missing, "DB_USER")
	}
	if cfg.DBPassword == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if cfg.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}

	// [Security] Reject weak/default secrets in production
	weakSecrets := []string{"dev-session-secret-change-me", "dev-jwt-secret-change-me", "changeme", "secret", ""}
	if !cfg.Debug {
		for _, weak := range weakSecrets {
			if cfg.SessionSecret == weak {
				log.Fatal("[Security][Config] FATAL: SESSION_SECRET is using a weak/default value. Set a strong secret (32+ chars) for production!")
			}
			if cfg.JWTSecret == weak {
				log.Fatal("[Security][Config] FATAL: JWT_SECRET is using a weak/default value. Set a strong secret (32+ chars) for production!")
			}
		}
		if len(cfg.SessionSecret) < 32 {
			log.Printf("[Security][Config] WARNING: SESSION_SECRET should be at least 32 characters for production")
		}
		if len(cfg.JWTSecret) < 32 {
			log.Printf("[Security][Config] WARNING: JWT_SECRET should be at least 32 characters for production")
		}
	}

	if len(missing) > 0 {
		log.Printf("[Security][Config] WARNING: Environment variables belum di-set: %v", missing)
		log.Printf("[Security][Config] Set variabel di atas via environment untuk production!")
		// Fallback untuk development — JANGAN dipakai di production
		if cfg.DBUser == "" {
			cfg.DBUser = "postgres"
		}
		if cfg.DBPassword == "" {
			cfg.DBPassword = "postgres"
		}
		if cfg.SessionSecret == "" {
			cfg.SessionSecret = "dev-session-secret-change-me"
		}
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = "dev-jwt-secret-change-me"
		}
	}

	return cfg
}
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
