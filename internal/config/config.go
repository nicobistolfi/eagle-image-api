// Package config loads the Eagle Image API runtime settings from environment
// variables.
//
// Every setting has a default, so the service starts with no environment set
// at all. Values that fail to parse fall back to their default rather than
// aborting startup: a malformed QUALITY should not take the service down.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment
// variables. See [FromEnv] for the variable names and defaults.
type Config struct {
	// Environment names the deployment, e.g. "production" or "development".
	Environment string
	// APIEndpoint is the request path that serves image transformations.
	APIEndpoint string
	// Port is the TCP port used when running outside AWS Lambda.
	Port int
	// Quality is the default output quality, 0-100, for lossy encoders.
	Quality int
	// Fit is the default resize mode when a request omits the fit parameter.
	Fit string
	// LogLevel is the verbosity passed to the logger package.
	LogLevel string
	// OriginWhitelist is the raw comma-separated allowlist, or "*" for any.
	OriginWhitelist string
	// AllowAllOrigins reports whether OriginWhitelist was "*".
	AllowAllOrigins bool
	// Origins holds the parsed, trimmed allowlist. Empty when AllowAllOrigins.
	Origins []string
	// RedirectOnError sends a 302 to the source image instead of an error.
	RedirectOnError bool
	// WebP enables WebP output for clients that accept it.
	WebP bool
	// AVIF enables AVIF output for clients that accept it.
	AVIF bool
	// AVIFMaxMP caps the megapixel area eligible for AVIF encoding, which is
	// slow enough on large images to risk a Lambda timeout.
	AVIFMaxMP float64
}

// Cfg is the global configuration instance populated by [Load].
var Cfg Config

// FromEnv builds a Config from the process environment.
//
// The variables read, with their defaults, are: ENVIRONMENT (production),
// API_ENDPOINT (/api/v1/image), PORT (3000), QUALITY (80), FIT (outside),
// LOG_LEVEL (silly), ORIGIN_WHITELIST (*), REDIRECT_ON_ERROR (false),
// WEBP (true), AVIF (true), and AVIF_MAX_MP (2).
//
// ORIGIN_WHITELIST accepts "*" to allow every origin, or a comma-separated
// list of hostnames; surrounding whitespace on each entry is trimmed.
func FromEnv() Config {
	c := Config{
		Environment:     envOrDefault("ENVIRONMENT", "production"),
		APIEndpoint:     envOrDefault("API_ENDPOINT", "/api/v1/image"),
		Port:            envOrDefaultInt("PORT", 3000),
		Quality:         envOrDefaultInt("QUALITY", 80),
		Fit:             envOrDefault("FIT", "outside"),
		LogLevel:        envOrDefault("LOG_LEVEL", "silly"),
		OriginWhitelist: envOrDefault("ORIGIN_WHITELIST", "*"),
		RedirectOnError: envOrDefaultBool("REDIRECT_ON_ERROR", false),
		WebP:            envOrDefaultBool("WEBP", true),
		AVIF:            envOrDefaultBool("AVIF", true),
		AVIFMaxMP:       envOrDefaultFloat("AVIF_MAX_MP", 2),
	}

	if c.OriginWhitelist == "*" {
		c.AllowAllOrigins = true
	} else {
		c.Origins = strings.Split(c.OriginWhitelist, ",")
		for i := range c.Origins {
			c.Origins[i] = strings.TrimSpace(c.Origins[i])
		}
	}

	return c
}

// Load reads the environment and stores the result in the global [Cfg].
func Load() {
	Cfg = FromEnv()
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
