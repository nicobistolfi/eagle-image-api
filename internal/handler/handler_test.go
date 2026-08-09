package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	stdimage "image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/nicobistolfi/eagle-image-api/internal/config"
)

// testPNG encodes a small gradient PNG. Generating it rather than embedding a
// blob keeps the fixture readable and guarantees libvips gets real pixel data.
func testPNG(t *testing.T) []byte {
	t.Helper()

	const size = 32
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test PNG: %v", err)
	}
	return buf.Bytes()
}

func TestMain(m *testing.M) {
	vips.LoggingSettings(nil, vips.LogLevelError)
	if err := vips.Startup(nil); err != nil {
		panic("starting vips: " + err.Error())
	}
	code := m.Run()
	vips.Shutdown()
	os.Exit(code)
}

// setConfig installs a test configuration and restores the previous one.
func setConfig(t *testing.T, c config.Config) {
	t.Helper()
	original := config.Cfg
	config.Cfg = c
	t.Cleanup(func() { config.Cfg = original })
}

func defaultTestConfig() config.Config {
	return config.Config{
		Environment:     "test",
		APIEndpoint:     "/api/v1/image",
		Quality:         80,
		Fit:             "outside",
		LogLevel:        "error",
		OriginWhitelist: "*",
		AllowAllOrigins: true,
		WebP:            true,
		AVIF:            true,
		AVIFMaxMP:       2,
	}
}

// imageServer serves the test PNG with the given content type.
func imageServer(t *testing.T, contentType string) *httptest.Server {
	t.Helper()
	png := testPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(png)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHandleRejectsNonGET(t *testing.T) {
	setConfig(t, defaultTestConfig())

	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
				HTTPMethod: method,
				Path:       "/api/v1/image",
			})
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", resp.StatusCode)
			}
		})
	}
}

func TestHandleHealth(t *testing.T) {
	setConfig(t, defaultTestConfig())

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/health",
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Body != "\U0001F985" {
		t.Errorf("body = %q, want the eagle emoji", resp.Body)
	}
}

func TestHandleUnknownPath(t *testing.T) {
	setConfig(t, defaultTestConfig())

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/nope",
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHandleHonoursConfiguredEndpoint verifies the route follows
// API_ENDPOINT rather than a hardcoded path.
func TestHandleHonoursConfiguredEndpoint(t *testing.T) {
	c := defaultTestConfig()
	c.APIEndpoint = "/custom/img"
	setConfig(t, c)

	srv := imageServer(t, "image/png")

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/custom/img",
		QueryStringParameters: map[string]string{"url": srv.URL + "/a.png"},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", resp.StatusCode, resp.Body)
	}

	// The old default path must now 404.
	resp, err = Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/api/v1/image",
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for the unconfigured path", resp.StatusCode)
	}
}

func TestProcessImageMissingURL(t *testing.T) {
	setConfig(t, defaultTestConfig())

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(resp.Body, "url") {
		t.Errorf("body = %q, want it to name the missing parameter", resp.Body)
	}
}

func TestProcessImageSuccess(t *testing.T) {
	setConfig(t, defaultTestConfig())
	srv := imageServer(t, "image/png")

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": srv.URL + "/a.png"},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, resp.Body)
	}
	if !resp.IsBase64Encoded {
		t.Error("IsBase64Encoded = false, want true")
	}
	if _, err := base64.StdEncoding.DecodeString(resp.Body); err != nil {
		t.Errorf("body is not valid base64: %v", err)
	}
	if resp.Headers["Content-Type"] == "" {
		t.Error("expected a Content-Type header")
	}
	if resp.Headers["Cache-Control"] == "" {
		t.Error("expected a Cache-Control header")
	}
}

// TestProcessImageNegotiatesWebP checks the Accept header is honoured and
// that the lookup is case-insensitive, since API Gateway does not normalise
// header casing.
func TestProcessImageNegotiatesWebP(t *testing.T) {
	setConfig(t, defaultTestConfig())
	srv := imageServer(t, "image/png")

	for _, headerName := range []string{"Accept", "accept", "ACCEPT"} {
		t.Run(headerName, func(t *testing.T) {
			resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
				HTTPMethod:            "GET",
				Path:                  "/api/v1/image",
				Headers:               map[string]string{headerName: "image/webp,*/*"},
				QueryStringParameters: map[string]string{"url": srv.URL + "/a.png"},
			})
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, resp.Body)
			}
			if resp.Headers["Content-Type"] != "image/webp" {
				t.Errorf("Content-Type = %q, want image/webp", resp.Headers["Content-Type"])
			}
		})
	}
}

func TestProcessImageNonImageURL(t *testing.T) {
	setConfig(t, defaultTestConfig())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": srv.URL + "/page.html"},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a non-image URL", resp.StatusCode)
	}
}

func TestProcessImageUnreachableURL(t *testing.T) {
	setConfig(t, defaultTestConfig())

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL + "/gone.png"
	srv.Close()

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": url},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestProcessImageOriginNotAllowed(t *testing.T) {
	c := defaultTestConfig()
	c.AllowAllOrigins = false
	c.OriginWhitelist = "allowed.example.com"
	c.Origins = []string{"allowed.example.com"}
	setConfig(t, c)

	srv := imageServer(t, "image/png")

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": srv.URL + "/a.png"},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a disallowed origin", resp.StatusCode)
	}
	if !strings.Contains(resp.Body, "Origin not allowed") {
		t.Errorf("body = %q, want the origin rejection message", resp.Body)
	}
}

func TestProcessImageCorruptImageData(t *testing.T) {
	setConfig(t, defaultTestConfig())

	// Claims to be an image but is not decodable, so the failure happens
	// inside libvips rather than at fetch time.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("definitely not a png"))
	}))
	defer srv.Close()

	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": srv.URL + "/broken.png"},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for undecodable image data", resp.StatusCode)
	}
}

func TestRedirectOnError(t *testing.T) {
	c := defaultTestConfig()
	c.RedirectOnError = true
	setConfig(t, c)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not a real png"))
	}))
	defer srv.Close()

	source := srv.URL + "/broken.png"
	resp, err := Handle(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		Path:                  "/api/v1/image",
		QueryStringParameters: map[string]string{"url": source},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if resp.Headers["Location"] != source {
		t.Errorf("Location = %q, want %q", resp.Headers["Location"], source)
	}
}

// TestRedirectOnErrorWithoutURL covers the case where redirect is enabled but
// there is no source URL to redirect to; a 400 is the only sensible answer.
func TestRedirectOnErrorWithoutURL(t *testing.T) {
	c := defaultTestConfig()
	c.RedirectOnError = true
	setConfig(t, c)

	resp := handleError(events.APIGatewayProxyRequest{
		QueryStringParameters: map[string]string{},
	}, errAny{})

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when there is no URL to redirect to", resp.StatusCode)
	}
}

type errAny struct{}

func (errAny) Error() string { return "synthetic failure" }

func TestFindAcceptHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"canonical", map[string]string{"Accept": "image/webp"}, "image/webp"},
		{"lowercase", map[string]string{"accept": "image/avif"}, "image/avif"},
		{"mixed case", map[string]string{"AcCePt": "image/png"}, "image/png"},
		{"absent", map[string]string{"User-Agent": "curl"}, ""},
		{"nil map", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findAcceptHeader(tt.headers); got != tt.want {
				t.Errorf("findAcceptHeader(%v) = %q, want %q", tt.headers, got, tt.want)
			}
		})
	}
}
