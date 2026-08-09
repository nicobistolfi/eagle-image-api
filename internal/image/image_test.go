package image

import (
	"bytes"
	"errors"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/nicobistolfi/eagle-image-api/internal/config"
)

func TestMain(m *testing.M) {
	vips.LoggingSettings(nil, vips.LogLevelError)
	if err := vips.Startup(nil); err != nil {
		panic("starting vips: " + err.Error())
	}
	code := m.Run()
	vips.Shutdown()
	os.Exit(code)
}

// --- fixtures --------------------------------------------------------------

// makePNG builds a w x h gradient PNG. Real pixel data matters here: libvips
// decodes lazily, so a malformed fixture only fails later, at export time.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / max(w-1, 1)),
				G: uint8((y * 255) / max(h-1, 1)),
				B: 96,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding PNG: %v", err)
	}
	return buf.Bytes()
}

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encoding JPEG: %v", err)
	}
	return buf.Bytes()
}

// newVipsImage loads a fixture into libvips for the low-level resize and
// operation tests.
func newVipsImage(t *testing.T, w, h int) *vips.ImageRef {
	t.Helper()
	ref, err := vips.NewImageFromBuffer(makePNG(t, w, h))
	if err != nil {
		t.Fatalf("loading fixture into vips: %v", err)
	}
	t.Cleanup(ref.Close)
	return ref
}

func testConfig() config.Config {
	return config.Config{
		Environment:     "test",
		APIEndpoint:     "/api/v1/image",
		Port:            3000,
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

// setConfig installs a test configuration and restores the previous one.
func setConfig(t *testing.T, c config.Config) {
	t.Helper()
	original := config.Cfg
	config.Cfg = c
	t.Cleanup(func() { config.Cfg = original })
}

func init() {
	config.Cfg = testConfig()
}

// pngServer serves a generated PNG over HTTP.
func pngServer(t *testing.T, w, h int) *httptest.Server {
	t.Helper()
	data := makePNG(t, w, h)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "image/png")
		_, _ = rw.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- query parameters ------------------------------------------------------

func TestParseQueryParams(t *testing.T) {
	m := map[string]string{
		"url":          "https://example.com/image.jpg",
		"width":        "200",
		"height":       "100",
		"fit":          "cover",
		"position":     "attention",
		"quality":      "90",
		"blur":         "5.5",
		"sharpen":      "3.5",
		"flip":         "",
		"flop":         "",
		"rotate":       "90",
		"alphaQuality": "70",
		"loop":         "3",
		"delay":        "120",
		"webpEffort":   "5",
		"background":   "#ffffff",
		"kernel":       "lanczos3",
	}

	p := ParseQueryParams(m)

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"URL", p.URL, "https://example.com/image.jpg"},
		{"Width", p.Width, 200},
		{"Height", p.Height, 100},
		{"Fit", p.Fit, "cover"},
		{"Position", p.Position, "attention"},
		{"Quality", p.Quality, 90},
		{"Blur", p.Blur, 5.5},
		{"Sharpen", p.Sharpen, 3.5},
		{"Flip", p.Flip, true},
		{"Flop", p.Flop, true},
		{"Rotate", p.Rotate, 90},
		{"AlphaQuality", p.AlphaQuality, 70},
		{"Loop", p.Loop, 3},
		{"Delay", p.Delay, 120},
		{"WebpEffort", p.WebpEffort, 5},
		{"Background", p.Background, "#ffffff"},
		{"Kernel", p.Kernel, "lanczos3"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseQueryParamsLossless(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"anything else", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			p := ParseQueryParams(map[string]string{"lossless": tt.value})
			if p.Lossless == nil {
				t.Fatal("Lossless = nil, want a value when the parameter is present")
			}
			if *p.Lossless != tt.want {
				t.Errorf("Lossless = %v, want %v", *p.Lossless, tt.want)
			}
		})
	}
}

func TestParseQueryParamsEmpty(t *testing.T) {
	p := ParseQueryParams(map[string]string{"url": "https://example.com/image.jpg"})

	if p.Width != 0 || p.Height != 0 || p.Quality != 0 {
		t.Errorf("expected zero dimensions and quality, got %+v", p)
	}
	if p.Flip || p.Flop {
		t.Error("expected flip and flop to be false when absent")
	}
	// nil rather than false: absent must stay distinguishable from
	// lossless=false so the encoder default is preserved.
	if p.Lossless != nil {
		t.Error("Lossless should be nil when the parameter is absent")
	}
}

// TestParseQueryParamsIgnoresMalformedNumbers documents that a bad numeric
// value degrades to the zero value rather than failing the request.
func TestParseQueryParamsIgnoresMalformedNumbers(t *testing.T) {
	p := ParseQueryParams(map[string]string{
		"width":   "wide",
		"height":  "",
		"quality": "high",
		"blur":    "lots",
		"rotate":  "sideways",
	})

	if p.Width != 0 || p.Height != 0 || p.Quality != 0 || p.Blur != 0 || p.Rotate != 0 {
		t.Errorf("expected malformed values to fall back to zero, got %+v", p)
	}
}

// --- helpers ---------------------------------------------------------------

func TestExtractDomain(t *testing.T) {
	tests := []struct{ url, domain string }{
		{"https://example.com/path/image.jpg", "example.com"},
		{"http://cdn.example.com/img.png", "cdn.example.com"},
		{"https://example.com:8080/img.png", "example.com:8080"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		if got := extractDomain(tt.url); got != tt.domain {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.domain)
		}
	}
}

func TestParseInterest(t *testing.T) {
	tests := []struct {
		position string
		want     vips.Interesting
	}{
		{"centre", vips.InterestingCentre},
		{"center", vips.InterestingCentre},
		{"CENTER", vips.InterestingCentre},
		{"entropy", vips.InterestingEntropy},
		{"attention", vips.InterestingAttention},
		{"", vips.InterestingCentre},
		{"nonsense", vips.InterestingCentre},
	}

	for _, tt := range tests {
		if got := parseInterest(tt.position); got != tt.want {
			t.Errorf("parseInterest(%q) = %v, want %v", tt.position, got, tt.want)
		}
	}
}

func TestNew(t *testing.T) {
	img := New("https://example.com/a.png")
	if img.URL != "https://example.com/a.png" {
		t.Errorf("URL = %q", img.URL)
	}
	if img.Data != nil {
		t.Error("expected no data before Load")
	}
}

func TestResponseHeaders(t *testing.T) {
	img := &Image{ContentType: "image/webp", Data: make([]byte, 1024)}

	h := img.ResponseHeaders()

	if h["Content-Type"] != "image/webp" {
		t.Errorf("Content-Type = %q", h["Content-Type"])
	}
	if h["Content-Length"] != "1024" {
		t.Errorf("Content-Length = %q, want 1024", h["Content-Length"])
	}
	if h["Cache-Control"] != "public, max-age=31536000" {
		t.Errorf("Cache-Control = %q", h["Cache-Control"])
	}
	if h["X-Powered-By"] != "Image API" {
		t.Errorf("X-Powered-By = %q", h["X-Powered-By"])
	}
	for _, k := range []string{"Expires", "Last-Modified", "Pragma"} {
		if h[k] == "" {
			t.Errorf("missing %s header", k)
		}
	}
}

func TestBase64(t *testing.T) {
	img := &Image{Data: []byte("hello")}
	if got := img.Base64(); got != "aGVsbG8=" {
		t.Errorf("Base64() = %q, want aGVsbG8=", got)
	}
}

// --- fetching --------------------------------------------------------------

func TestIsImage(t *testing.T) {
	srv := pngServer(t, 8, 8)

	ok, err := New(srv.URL + "/a.png").IsImage()
	if err != nil {
		t.Fatalf("IsImage() error: %v", err)
	}
	if !ok {
		t.Error("IsImage() = false, want true for an image/png response")
	}
}

func TestIsImageNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}))
	defer srv.Close()

	ok, err := New(srv.URL).IsImage()
	if err != nil {
		t.Fatalf("IsImage() error: %v", err)
	}
	if ok {
		t.Error("IsImage() = true, want false for text/html")
	}
}

func TestIsImageErrors(t *testing.T) {
	t.Run("malformed url", func(t *testing.T) {
		if _, err := New("://not a url").IsImage(); err == nil {
			t.Fatal("expected an error for a malformed URL")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()

		if _, err := New(url).IsImage(); err == nil {
			t.Fatal("expected an error when the host is unreachable")
		}
	})
}

func TestLoad(t *testing.T) {
	setConfig(t, testConfig())
	srv := pngServer(t, 16, 16)

	img := New(srv.URL + "/a.png")
	if err := img.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(img.Data) == 0 {
		t.Error("expected image data after Load")
	}
	if img.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", img.ContentType)
	}
}

func TestLoadOriginNotAllowed(t *testing.T) {
	c := testConfig()
	c.AllowAllOrigins = false
	c.Origins = []string{"allowed.example.com"}
	setConfig(t, c)

	err := New("https://blocked.example.com/a.png").Load()
	if err == nil {
		t.Fatal("expected an error for a disallowed origin")
	}
	if !errors.Is(err, ErrOriginNotAllowed) {
		t.Errorf("error = %v, want ErrOriginNotAllowed", err)
	}
}

func TestLoadOriginAllowedByWhitelist(t *testing.T) {
	srv := pngServer(t, 8, 8)

	c := testConfig()
	c.AllowAllOrigins = false
	c.Origins = []string{strings.TrimPrefix(srv.URL, "http://")}
	setConfig(t, c)

	if err := New(srv.URL + "/a.png").Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	setConfig(t, testConfig())

	t.Run("malformed url", func(t *testing.T) {
		if err := New("://not a url").Load(); err == nil {
			t.Fatal("expected an error for a malformed URL")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()

		if err := New(url).Load(); err == nil {
			t.Fatal("expected an error when the host is unreachable")
		}
	})
}

// --- resizing --------------------------------------------------------------

func TestResizeFitModes(t *testing.T) {
	setConfig(t, testConfig())

	// The 200x100 source has a 2:1 aspect ratio, which makes every mode
	// produce a distinguishable result for a 50x50 request.
	tests := []struct {
		name         string
		params       QueryParams
		wantW, wantH int
	}{
		{"cover crops to exact size", QueryParams{Width: 50, Height: 50, Fit: "cover"}, 50, 50},
		{"fill forces exact size", QueryParams{Width: 60, Height: 30, Fit: "fill"}, 60, 30},
		{"contain fits inside", QueryParams{Width: 50, Height: 50, Fit: "contain"}, 50, 25},
		{"inside behaves like contain", QueryParams{Width: 50, Height: 50, Fit: "inside"}, 50, 25},
		{"outside covers the box", QueryParams{Width: 50, Height: 50, Fit: "outside"}, 100, 50},
		{"unknown fit falls back to contain", QueryParams{Width: 50, Height: 50, Fit: "bogus"}, 50, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := newVipsImage(t, 200, 100)
			img := &Image{}

			if err := img.resize(ref, tt.params); err != nil {
				t.Fatalf("resize() error: %v", err)
			}
			if ref.Width() != tt.wantW || ref.Height() != tt.wantH {
				t.Errorf("size = %dx%d, want %dx%d", ref.Width(), ref.Height(), tt.wantW, tt.wantH)
			}
			if img.Width != ref.Width() || img.Height != ref.Height() {
				t.Errorf("Image dimensions %dx%d out of sync with vips %dx%d",
					img.Width, img.Height, ref.Width(), ref.Height())
			}
		})
	}
}

func TestResizeNoDimensionsIsNoOp(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 120, 80)
	img := &Image{}

	if err := img.resize(ref, QueryParams{}); err != nil {
		t.Fatalf("resize() error: %v", err)
	}
	if ref.Width() != 120 || ref.Height() != 80 {
		t.Errorf("size = %dx%d, want the original 120x80", ref.Width(), ref.Height())
	}
	if img.Width != 0 || img.Height != 0 {
		t.Error("expected dimensions to stay unset when no resize happens")
	}
}

func TestResizeUsesConfiguredDefaultFit(t *testing.T) {
	c := testConfig()
	c.Fit = "fill"
	setConfig(t, c)

	ref := newVipsImage(t, 200, 100)
	img := &Image{}

	// No fit in the request, so the configured default must apply.
	if err := img.resize(ref, QueryParams{Width: 40, Height: 40}); err != nil {
		t.Fatalf("resize() error: %v", err)
	}
	if ref.Width() != 40 || ref.Height() != 40 {
		t.Errorf("size = %dx%d, want 40x40 from the configured fill default", ref.Width(), ref.Height())
	}
}

func TestResizeSingleDimension(t *testing.T) {
	setConfig(t, testConfig())

	tests := []struct {
		name         string
		params       QueryParams
		wantW, wantH int
	}{
		{"cover width only", QueryParams{Width: 100, Fit: "cover"}, 100, 50},
		{"cover height only", QueryParams{Height: 25, Fit: "cover"}, 50, 25},
		{"contain width only", QueryParams{Width: 100, Fit: "contain"}, 100, 50},
		{"contain height only", QueryParams{Height: 25, Fit: "contain"}, 50, 25},
		{"outside width only", QueryParams{Width: 100, Fit: "outside"}, 100, 50},
		{"outside height only", QueryParams{Height: 25, Fit: "outside"}, 50, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := newVipsImage(t, 200, 100)
			img := &Image{}

			if err := img.resize(ref, tt.params); err != nil {
				t.Fatalf("resize() error: %v", err)
			}
			if ref.Width() != tt.wantW || ref.Height() != tt.wantH {
				t.Errorf("size = %dx%d, want %dx%d", ref.Width(), ref.Height(), tt.wantW, tt.wantH)
			}
		})
	}
}

// TestFillNeedsBothDimensions documents that fill requires a width and a
// height; with only one it leaves the image untouched.
func TestFillNeedsBothDimensions(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 200, 100)
	img := &Image{}

	if err := img.resize(ref, QueryParams{Width: 50, Fit: "fill"}); err != nil {
		t.Fatalf("resize() error: %v", err)
	}
	if ref.Width() != 200 || ref.Height() != 100 {
		t.Errorf("size = %dx%d, want the original 200x100", ref.Width(), ref.Height())
	}
}

func TestResizeHelpersWithNoTarget(t *testing.T) {
	ref := newVipsImage(t, 40, 40)

	if err := resizeContain(ref, 0, 0); err != nil {
		t.Fatalf("resizeContain() error: %v", err)
	}
	if err := resizeOutside(ref, 0, 0, 40, 40); err != nil {
		t.Fatalf("resizeOutside() error: %v", err)
	}
	if ref.Width() != 40 || ref.Height() != 40 {
		t.Errorf("size = %dx%d, want the original 40x40", ref.Width(), ref.Height())
	}
}

func TestCoverWithPosition(t *testing.T) {
	setConfig(t, testConfig())

	for _, position := range []string{"centre", "entropy", "attention", ""} {
		name := position
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			ref := newVipsImage(t, 200, 100)
			img := &Image{}

			err := img.resize(ref, QueryParams{Width: 40, Height: 40, Fit: "cover", Position: position})
			if err != nil {
				t.Fatalf("resize() error: %v", err)
			}
			if ref.Width() != 40 || ref.Height() != 40 {
				t.Errorf("size = %dx%d, want 40x40", ref.Width(), ref.Height())
			}
		})
	}
}

// --- operations ------------------------------------------------------------

func TestApplyOperations(t *testing.T) {
	tests := []struct {
		name   string
		params QueryParams
	}{
		{"blur", QueryParams{Blur: 3}},
		{"sharpen", QueryParams{Sharpen: 5}},
		{"flip", QueryParams{Flip: true}},
		{"flop", QueryParams{Flop: true}},
		{"flip and flop", QueryParams{Flip: true, Flop: true}},
		{"rotate 90", QueryParams{Rotate: 90}},
		{"rotate 180", QueryParams{Rotate: 180}},
		{"rotate 270", QueryParams{Rotate: 270}},
		{"rotate arbitrary", QueryParams{Rotate: 45}},
		{"no operations", QueryParams{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := newVipsImage(t, 40, 20)
			img := &Image{}

			if err := img.applyOperations(ref, tt.params); err != nil {
				t.Fatalf("applyOperations() error: %v", err)
			}
		})
	}
}

func TestRotate90SwapsDimensions(t *testing.T) {
	ref := newVipsImage(t, 40, 20)
	img := &Image{}

	if err := img.applyOperations(ref, QueryParams{Rotate: 90}); err != nil {
		t.Fatalf("applyOperations() error: %v", err)
	}
	if ref.Width() != 20 || ref.Height() != 40 {
		t.Errorf("size = %dx%d, want 20x40 after a 90 degree rotation", ref.Width(), ref.Height())
	}
}

// TestOperationsOutOfRangeAreSkipped covers the guard rails: values outside
// the accepted range are ignored rather than handed to libvips.
func TestOperationsOutOfRangeAreSkipped(t *testing.T) {
	tests := []struct {
		name   string
		params QueryParams
	}{
		{"blur above range", QueryParams{Blur: 500}},
		{"blur zero", QueryParams{Blur: 0}},
		{"blur negative", QueryParams{Blur: -5}},
		{"sharpen at lower bound", QueryParams{Sharpen: 1}},
		{"sharpen above range", QueryParams{Sharpen: 500}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := newVipsImage(t, 20, 20)
			img := &Image{}

			if err := img.applyOperations(ref, tt.params); err != nil {
				t.Fatalf("applyOperations() error: %v", err)
			}
		})
	}
}

// --- format conversion -----------------------------------------------------

func TestConvertFormatNative(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	if err := img.convertFormat(ref, QueryParams{}, ""); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want the original image/png", img.ContentType)
	}
	if len(img.Data) == 0 {
		t.Error("expected encoded data")
	}
}

func TestConvertFormatWebP(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	if err := img.convertFormat(ref, QueryParams{}, "image/webp,*/*"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", img.ContentType)
	}
}

func TestConvertFormatModernFormatsDisabled(t *testing.T) {
	c := testConfig()
	c.WebP = false
	c.AVIF = false
	setConfig(t, c)

	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	if err := img.convertFormat(ref, QueryParams{}, "image/webp,image/avif"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png when WebP and AVIF are disabled", img.ContentType)
	}
}

func TestConvertFormatWebPWithOptions(t *testing.T) {
	setConfig(t, testConfig())
	lossless := true
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	params := QueryParams{Quality: 60, Lossless: &lossless, WebpEffort: 4}
	if err := img.convertFormat(ref, params, "image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", img.ContentType)
	}
}

// TestConvertFormatGifSkipsAVIF checks animated content is not sent through
// the AVIF encoder, falling through to WebP instead.
func TestConvertFormatGifSkipsAVIF(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/gif"}

	if err := img.convertFormat(ref, QueryParams{}, "image/avif,image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp for a GIF source", img.ContentType)
	}
}

// TestConvertFormatAVIFSkippedWhenTooLarge covers the megapixel guard that
// keeps slow AVIF encodes from timing out the Lambda.
func TestConvertFormatAVIFSkippedWhenTooLarge(t *testing.T) {
	c := testConfig()
	c.AVIFMaxMP = 0.0001 // any real image exceeds this
	setConfig(t, c)

	ref := newVipsImage(t, 100, 100)
	img := &Image{ContentType: "image/png"}

	if err := img.convertFormat(ref, QueryParams{}, "image/avif,image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp above the AVIF limit", img.ContentType)
	}
}

// TestConvertFormatAVIFSkippedForLargeRequestedSize checks the guard also
// considers the requested output size, not just the current one.
func TestConvertFormatAVIFSkippedForLargeRequestedSize(t *testing.T) {
	c := testConfig()
	c.AVIFMaxMP = 1
	setConfig(t, c)

	ref := newVipsImage(t, 20, 20) // small right now...
	img := &Image{ContentType: "image/png"}

	// ...but 2000x2000 was requested, which is 4 MP.
	params := QueryParams{Width: 2000, Height: 2000}
	if err := img.convertFormat(ref, params, "image/avif,image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp above the AVIF limit", img.ContentType)
	}
}

func TestConvertFormatAVIF(t *testing.T) {
	setConfig(t, testConfig())
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	err := img.convertFormat(ref, QueryParams{Quality: 50, WebpEffort: 6}, "image/avif,image/webp")
	if err != nil {
		// Not every libvips build ships an AVIF encoder.
		t.Skipf("AVIF encoding unavailable in this libvips build: %v", err)
	}
	if img.ContentType != "image/avif" {
		t.Errorf("ContentType = %q, want image/avif", img.ContentType)
	}
	if len(img.Data) == 0 {
		t.Error("expected encoded AVIF data")
	}
}

func TestConvertFormatAVIFLossless(t *testing.T) {
	setConfig(t, testConfig())
	lossless := true
	ref := newVipsImage(t, 20, 20)
	img := &Image{ContentType: "image/png"}

	err := img.convertFormat(ref, QueryParams{Lossless: &lossless}, "image/avif")
	if err != nil {
		t.Skipf("AVIF encoding unavailable in this libvips build: %v", err)
	}
	if img.ContentType != "image/avif" {
		t.Errorf("ContentType = %q, want image/avif", img.ContentType)
	}
}

// TestConvertFormatDefaultQuality checks the configured quality applies when
// the request does not carry one.
func TestConvertFormatDefaultQuality(t *testing.T) {
	c := testConfig()
	c.Quality = 10
	setConfig(t, c)

	ref := newVipsImage(t, 80, 80)
	lowQ := &Image{ContentType: "image/png"}
	if err := lowQ.convertFormat(ref, QueryParams{}, "image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}

	ref2 := newVipsImage(t, 80, 80)
	highQ := &Image{ContentType: "image/png"}
	if err := highQ.convertFormat(ref2, QueryParams{Quality: 95}, "image/webp"); err != nil {
		t.Fatalf("convertFormat() error: %v", err)
	}

	if len(lowQ.Data) >= len(highQ.Data) {
		t.Errorf("quality 10 produced %d bytes and quality 95 produced %d; expected the configured default to be applied",
			len(lowQ.Data), len(highQ.Data))
	}
}

// --- end to end ------------------------------------------------------------

func TestProcess(t *testing.T) {
	setConfig(t, testConfig())

	img := &Image{ContentType: "image/png", Data: makePNG(t, 200, 100)}
	params := QueryParams{Width: 50, Height: 50, Fit: "cover", Blur: 2, Rotate: 90}

	if err := img.Process(params, "image/webp"); err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", img.ContentType)
	}
	if len(img.Data) == 0 {
		t.Error("expected processed data")
	}
}

func TestProcessJPEG(t *testing.T) {
	setConfig(t, testConfig())

	img := &Image{ContentType: "image/jpeg", Data: makeJPEG(t, 120, 80)}
	if err := img.Process(QueryParams{Width: 40}, ""); err != nil {
		t.Fatalf("Process() error: %v", err)
	}
	if img.Width != 40 {
		t.Errorf("Width = %d, want 40", img.Width)
	}
}

func TestProcessInvalidData(t *testing.T) {
	setConfig(t, testConfig())

	img := &Image{ContentType: "image/png", Data: []byte("not an image at all")}
	err := img.Process(QueryParams{}, "")
	if err == nil {
		t.Fatal("expected an error for undecodable data")
	}
	if !strings.Contains(err.Error(), "loading image into vips") {
		t.Errorf("error = %v, want it to identify the load step", err)
	}
}

// TestLoadThenProcess exercises the full fetch-and-transform path a request
// takes, against a real HTTP server.
func TestLoadThenProcess(t *testing.T) {
	setConfig(t, testConfig())
	srv := pngServer(t, 300, 150)

	img := New(srv.URL + "/a.png")

	ok, err := img.IsImage()
	if err != nil {
		t.Fatalf("IsImage() error: %v", err)
	}
	if !ok {
		t.Fatal("IsImage() = false, want true")
	}
	if err := img.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := img.Process(QueryParams{Width: 100, Fit: "contain"}, "image/webp"); err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if img.Width != 100 || img.Height != 50 {
		t.Errorf("size = %dx%d, want 100x50", img.Width, img.Height)
	}
	if img.ContentType != "image/webp" {
		t.Errorf("ContentType = %q, want image/webp", img.ContentType)
	}
	if img.Base64() == "" {
		t.Error("expected a base64 body")
	}
	if img.ResponseHeaders()["Content-Type"] != "image/webp" {
		t.Error("response headers should reflect the negotiated content type")
	}
}
