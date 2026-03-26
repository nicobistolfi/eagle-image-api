package image

import (
	"testing"

	"github.com/zantez/image-api/internal/config"
)

func init() {
	config.Cfg = config.Config{
		Environment:     "test",
		APIEndpoint:     "/api/v1/image",
		Port:            3000,
		Quality:         80,
		Fit:             "outside",
		LogLevel:        "error",
		OriginWhitelist: "*",
		AllowAllOrigins: true,
		RedirectOnError: false,
		WebP:            true,
		AVIF:            true,
		AVIFMaxMP:       2,
	}
}

func TestParseQueryParams(t *testing.T) {
	m := map[string]string{
		"url":     "https://example.com/image.jpg",
		"width":   "200",
		"height":  "100",
		"fit":     "cover",
		"quality": "90",
		"blur":    "5.5",
		"flip":    "",
		"rotate":  "90",
	}

	p := ParseQueryParams(m)

	if p.URL != "https://example.com/image.jpg" {
		t.Errorf("expected URL https://example.com/image.jpg, got %s", p.URL)
	}
	if p.Width != 200 {
		t.Errorf("expected width 200, got %d", p.Width)
	}
	if p.Height != 100 {
		t.Errorf("expected height 100, got %d", p.Height)
	}
	if p.Fit != "cover" {
		t.Errorf("expected fit cover, got %s", p.Fit)
	}
	if p.Quality != 90 {
		t.Errorf("expected quality 90, got %d", p.Quality)
	}
	if p.Blur != 5.5 {
		t.Errorf("expected blur 5.5, got %f", p.Blur)
	}
	if !p.Flip {
		t.Error("expected flip to be true")
	}
	if p.Rotate != 90 {
		t.Errorf("expected rotate 90, got %d", p.Rotate)
	}
}

func TestParseQueryParamsLossless(t *testing.T) {
	m := map[string]string{
		"url":      "https://example.com/image.jpg",
		"lossless": "true",
	}
	p := ParseQueryParams(m)
	if p.Lossless == nil || !*p.Lossless {
		t.Error("expected lossless to be true")
	}

	m["lossless"] = "0"
	p = ParseQueryParams(m)
	if p.Lossless == nil || *p.Lossless {
		t.Error("expected lossless to be false for '0'")
	}
}

func TestParseQueryParamsEmpty(t *testing.T) {
	m := map[string]string{
		"url": "https://example.com/image.jpg",
	}
	p := ParseQueryParams(m)
	if p.Width != 0 {
		t.Errorf("expected width 0, got %d", p.Width)
	}
	if p.Flip {
		t.Error("expected flip to be false")
	}
	if p.Lossless != nil {
		t.Error("expected lossless to be nil")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url    string
		domain string
	}{
		{"https://example.com/path/image.jpg", "example.com"},
		{"http://cdn.example.com/img.png", "cdn.example.com"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		got := extractDomain(tt.url)
		if got != tt.domain {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.domain)
		}
	}
}

func TestResponseHeaders(t *testing.T) {
	img := &Image{
		ContentType: "image/webp",
		Data:        make([]byte, 1024),
	}

	headers := img.ResponseHeaders()

	if headers["Content-Type"] != "image/webp" {
		t.Errorf("expected Content-Type image/webp, got %s", headers["Content-Type"])
	}
	if headers["Content-Length"] != "1024" {
		t.Errorf("expected Content-Length 1024, got %s", headers["Content-Length"])
	}
	if headers["Cache-Control"] != "public, max-age=31536000" {
		t.Errorf("unexpected Cache-Control: %s", headers["Cache-Control"])
	}
	if headers["X-Powered-By"] != "Image API" {
		t.Errorf("unexpected X-Powered-By: %s", headers["X-Powered-By"])
	}
}

func TestBase64(t *testing.T) {
	img := &Image{
		Data: []byte("hello"),
	}
	expected := "aGVsbG8="
	if got := img.Base64(); got != expected {
		t.Errorf("Base64() = %q, want %q", got, expected)
	}
}
