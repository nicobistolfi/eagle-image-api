package config

import (
	"reflect"
	"testing"
)

func TestFromEnvDefaults(t *testing.T) {
	// t.Setenv on every key with an empty value guarantees a clean slate
	// regardless of what the developer has exported locally.
	for _, k := range []string{
		"ENVIRONMENT", "API_ENDPOINT", "PORT", "QUALITY", "FIT", "LOG_LEVEL",
		"ORIGIN_WHITELIST", "REDIRECT_ON_ERROR", "WEBP", "AVIF", "AVIF_MAX_MP",
	} {
		t.Setenv(k, "")
	}

	c := FromEnv()

	want := Config{
		Environment:     "production",
		APIEndpoint:     "/api/v1/image",
		Port:            3000,
		Quality:         80,
		Fit:             "outside",
		LogLevel:        "silly",
		OriginWhitelist: "*",
		AllowAllOrigins: true,
		RedirectOnError: false,
		WebP:            true,
		AVIF:            true,
		AVIFMaxMP:       2,
	}

	if !reflect.DeepEqual(c, want) {
		t.Errorf("FromEnv() = %+v, want %+v", c, want)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("API_ENDPOINT", "/img")
	t.Setenv("PORT", "8080")
	t.Setenv("QUALITY", "55")
	t.Setenv("FIT", "cover")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("ORIGIN_WHITELIST", "example.com, cdn.example.com ,foo.test")
	t.Setenv("REDIRECT_ON_ERROR", "true")
	t.Setenv("WEBP", "false")
	t.Setenv("AVIF", "no")
	t.Setenv("AVIF_MAX_MP", "4.5")

	c := FromEnv()

	if c.Environment != "development" {
		t.Errorf("Environment = %q, want development", c.Environment)
	}
	if c.APIEndpoint != "/img" {
		t.Errorf("APIEndpoint = %q, want /img", c.APIEndpoint)
	}
	if c.Port != 8080 {
		t.Errorf("Port = %d, want 8080", c.Port)
	}
	if c.Quality != 55 {
		t.Errorf("Quality = %d, want 55", c.Quality)
	}
	if c.Fit != "cover" {
		t.Errorf("Fit = %q, want cover", c.Fit)
	}
	if c.LogLevel != "error" {
		t.Errorf("LogLevel = %q, want error", c.LogLevel)
	}
	if !c.RedirectOnError {
		t.Error("RedirectOnError = false, want true")
	}
	if c.WebP {
		t.Error("WebP = true, want false")
	}
	if c.AVIF {
		t.Error(`AVIF = true, want false for "no"`)
	}
	if c.AVIFMaxMP != 4.5 {
		t.Errorf("AVIFMaxMP = %v, want 4.5", c.AVIFMaxMP)
	}
	if c.AllowAllOrigins {
		t.Error("AllowAllOrigins = true, want false for an explicit list")
	}
	wantOrigins := []string{"example.com", "cdn.example.com", "foo.test"}
	if !reflect.DeepEqual(c.Origins, wantOrigins) {
		t.Errorf("Origins = %q, want %q", c.Origins, wantOrigins)
	}
}

func TestFromEnvSingleOrigin(t *testing.T) {
	t.Setenv("ORIGIN_WHITELIST", "only.example.com")

	c := FromEnv()

	if c.AllowAllOrigins {
		t.Error("AllowAllOrigins = true, want false")
	}
	if !reflect.DeepEqual(c.Origins, []string{"only.example.com"}) {
		t.Errorf("Origins = %q, want [only.example.com]", c.Origins)
	}
}

func TestFromEnvMalformedValuesFallBackToDefaults(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	t.Setenv("QUALITY", "12.5") // Atoi rejects floats
	t.Setenv("AVIF_MAX_MP", "huge")

	c := FromEnv()

	if c.Port != 3000 {
		t.Errorf("Port = %d, want default 3000 for malformed input", c.Port)
	}
	if c.Quality != 80 {
		t.Errorf("Quality = %d, want default 80 for malformed input", c.Quality)
	}
	if c.AVIFMaxMP != 2 {
		t.Errorf("AVIFMaxMP = %v, want default 2 for malformed input", c.AVIFMaxMP)
	}
}

func TestBoolParsing(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"maybe", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("REDIRECT_ON_ERROR", tt.value)
			if got := FromEnv().RedirectOnError; got != tt.want {
				t.Errorf("REDIRECT_ON_ERROR=%q gave %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestBoolUnsetKeepsFallback(t *testing.T) {
	// WEBP defaults to true and REDIRECT_ON_ERROR to false; an unset variable
	// must preserve each distinct fallback rather than coercing to false.
	t.Setenv("WEBP", "")
	t.Setenv("REDIRECT_ON_ERROR", "")

	c := FromEnv()

	if !c.WebP {
		t.Error("WebP = false, want default true when unset")
	}
	if c.RedirectOnError {
		t.Error("RedirectOnError = true, want default false when unset")
	}
}

func TestLoadPopulatesGlobal(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging")
	t.Setenv("QUALITY", "42")

	original := Cfg
	t.Cleanup(func() { Cfg = original })

	Load()

	if Cfg.Environment != "staging" {
		t.Errorf("Cfg.Environment = %q, want staging", Cfg.Environment)
	}
	if Cfg.Quality != 42 {
		t.Errorf("Cfg.Quality = %d, want 42", Cfg.Quality)
	}
}
