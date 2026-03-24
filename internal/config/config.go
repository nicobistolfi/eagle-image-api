package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Environment    string
	APIEndpoint    string
	Port           int
	Quality        int
	Fit            string
	LogLevel       string
	OriginWhitelist string
	AllowAllOrigins bool
	Origins        []string
	RedirectOnError bool
	WebP           bool
	AVIF           bool
	AVIFMaxMP      float64
}

// Cfg is the global configuration instance.
var Cfg Config

// Load reads environment variables and populates the global Config.
func Load() {
	Cfg = Config{
		Environment:    envOrDefault("ENVIRONMENT", "production"),
		APIEndpoint:    envOrDefault("API_ENDPOINT", "/api/v1/image"),
		Port:           envOrDefaultInt("PORT", 3000),
		Quality:        envOrDefaultInt("QUALITY", 80),
		Fit:            envOrDefault("FIT", "outside"),
		LogLevel:       envOrDefault("LOG_LEVEL", "silly"),
		OriginWhitelist: envOrDefault("ORIGIN_WHITELIST", "*"),
		RedirectOnError: envOrDefaultBool("REDIRECT_ON_ERROR", false),
		WebP:           envOrDefaultBool("WEBP", true),
		AVIF:           envOrDefaultBool("AVIF", true),
		AVIFMaxMP:      envOrDefaultFloat("AVIF_MAX_MP", 2),
	}

	if Cfg.OriginWhitelist == "*" {
		Cfg.AllowAllOrigins = true
	} else {
		Cfg.Origins = strings.Split(Cfg.OriginWhitelist, ",")
		for i := range Cfg.Origins {
			Cfg.Origins[i] = strings.TrimSpace(Cfg.Origins[i])
		}
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		v = strings.ToLower(v)
		return v == "true" || v == "1" || v == "yes"
	}
	return fallback
}

func envOrDefaultFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
