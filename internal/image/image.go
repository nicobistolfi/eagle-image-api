package image

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/zantez/image-api/internal/config"
	"github.com/zantez/image-api/internal/logger"
)

var defaultHeaders = map[string]string{
	"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36",
	"Accept":                    "*/*",
	"Accept-Encoding":           "gzip, deflate, br",
	"Connection":                "keep-alive",
	"Upgrade-Insecure-Requests": "1",
}

// QueryParams represents the image transformation parameters from the request.
type QueryParams struct {
	URL          string
	Width        int
	Height       int
	Fit          string
	Position     string
	Quality      int
	Lossless     *bool
	Blur         float64
	Sharpen      float64
	Flip         bool
	Flop         bool
	Rotate       int
	AlphaQuality int
	Loop         int
	Delay        int
	WebpEffort   int
	Background   string
	Kernel       string
}

// ParseQueryParams converts a map of string parameters to QueryParams.
func ParseQueryParams(m map[string]string) QueryParams {
	p := QueryParams{
		URL:      m["url"],
		Fit:      m["fit"],
		Position: m["position"],
		Background: m["background"],
		Kernel:   m["kernel"],
	}

	if v, ok := m["width"]; ok && v != "" {
		p.Width, _ = strconv.Atoi(v)
	}
	if v, ok := m["height"]; ok && v != "" {
		p.Height, _ = strconv.Atoi(v)
	}
	if v, ok := m["quality"]; ok && v != "" {
		p.Quality, _ = strconv.Atoi(v)
	}
	if v, ok := m["blur"]; ok && v != "" {
		p.Blur, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := m["sharpen"]; ok && v != "" {
		p.Sharpen, _ = strconv.ParseFloat(v, 64)
	}
	if _, ok := m["flip"]; ok {
		p.Flip = true
	}
	if _, ok := m["flop"]; ok {
		p.Flop = true
	}
	if v, ok := m["rotate"]; ok && v != "" {
		p.Rotate, _ = strconv.Atoi(v)
	}
	if v, ok := m["alphaQuality"]; ok && v != "" {
		p.AlphaQuality, _ = strconv.Atoi(v)
	}
	if v, ok := m["loop"]; ok && v != "" {
		p.Loop, _ = strconv.Atoi(v)
	}
	if v, ok := m["delay"]; ok && v != "" {
		p.Delay, _ = strconv.Atoi(v)
	}
	if v, ok := m["webpEffort"]; ok && v != "" {
		p.WebpEffort, _ = strconv.Atoi(v)
	}
	if v, ok := m["lossless"]; ok {
		b := strings.ToLower(v) == "true" || v == "1"
		p.Lossless = &b
	}

	return p
}

// Image handles fetching, processing, and converting images.
type Image struct {
	URL         string
	ContentType string
	Data        []byte
	Width       int
	Height      int
}

// New creates a new Image instance.
func New(url string) *Image {
	return &Image{URL: url}
}

// IsImage performs a HEAD request to verify the URL points to an image.
func (img *Image) IsImage() (bool, error) {
	req, err := http.NewRequest(http.MethodHead, img.URL, nil)
	if err != nil {
		return false, fmt.Errorf("creating HEAD request: %w", err)
	}
	for k, v := range defaultHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("HEAD request failed: %w", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "image"), nil
}

// Load fetches the image data from the URL, validating the origin whitelist.
func (img *Image) Load() error {
	cfg := &config.Cfg

	if !cfg.AllowAllOrigins {
		domain := extractDomain(img.URL)
		allowed := false
		for _, origin := range cfg.Origins {
			if origin == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("origin not allowed")
		}
	}

	req, err := http.NewRequest(http.MethodGet, img.URL, nil)
	if err != nil {
		return fmt.Errorf("creating GET request: %w", err)
	}
	for k, v := range defaultHeaders {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET request failed: %w", err)
	}
	defer resp.Body.Close()

	img.Data, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	img.ContentType = resp.Header.Get("Content-Type")
	return nil
}

// Process applies all transformations based on query parameters and Accept header.
func (img *Image) Process(params QueryParams, acceptHeader string) error {
	vImg, err := vips.NewImageFromBuffer(img.Data)
	if err != nil {
		return fmt.Errorf("loading image into vips: %w", err)
	}
	defer vImg.Close()

	if err := img.resize(vImg, params); err != nil {
		return fmt.Errorf("resize: %w", err)
	}

	if err := img.applyOperations(vImg, params); err != nil {
		return fmt.Errorf("operations: %w", err)
	}

	if err := img.convertFormat(vImg, params, acceptHeader); err != nil {
		return fmt.Errorf("format conversion: %w", err)
	}

	return nil
}

func (img *Image) resize(vImg *vips.ImageRef, p QueryParams) error {
	if p.Width == 0 && p.Height == 0 {
		return nil
	}

	origWidth := vImg.Width()
	origHeight := vImg.Height()

	fit := p.Fit
	if fit == "" {
		fit = config.Cfg.Fit
	}

	switch fit {
	case "cover":
		// Resize to cover target dimensions, then crop
		if err := resizeCover(vImg, p.Width, p.Height, p.Position); err != nil {
			return err
		}
	case "contain":
		// Resize to fit within target, maintaining aspect ratio
		if err := resizeContain(vImg, p.Width, p.Height); err != nil {
			return err
		}
	case "fill":
		// Force resize to exact dimensions
		if p.Width > 0 && p.Height > 0 {
			hScale := float64(p.Width) / float64(origWidth)
			vScale := float64(p.Height) / float64(origHeight)
			if err := vImg.ResizeWithVScale(hScale, vScale, vips.KernelLanczos3); err != nil {
				return err
			}
		}
	case "inside":
		// Same as contain: fit within dimensions
		if err := resizeContain(vImg, p.Width, p.Height); err != nil {
			return err
		}
	case "outside":
		// Resize so smallest side matches target
		if err := resizeOutside(vImg, p.Width, p.Height, origWidth, origHeight); err != nil {
			return err
		}
	default:
		// Default: simple thumbnail resize
		if p.Width > 0 || p.Height > 0 {
			if err := resizeContain(vImg, p.Width, p.Height); err != nil {
				return err
			}
		}
	}

	img.Width = vImg.Width()
	img.Height = vImg.Height()
	return nil
}

func resizeCover(vImg *vips.ImageRef, width, height int, position string) error {
	if width == 0 || height == 0 {
		// If only one dimension, just resize that dimension
		scale := 1.0
		if width > 0 {
			scale = float64(width) / float64(vImg.Width())
		} else {
			scale = float64(height) / float64(vImg.Height())
		}
		return vImg.Resize(scale, vips.KernelLanczos3)
	}

	// Resize to cover both dimensions
	hScale := float64(width) / float64(vImg.Width())
	vScale := float64(height) / float64(vImg.Height())
	scale := hScale
	if vScale > hScale {
		scale = vScale
	}

	if err := vImg.Resize(scale, vips.KernelLanczos3); err != nil {
		return err
	}

	// Crop to exact dimensions
	interest := parseInterest(position)
	return vImg.SmartCrop(width, height, interest)
}

func resizeContain(vImg *vips.ImageRef, width, height int) error {
	if width == 0 && height == 0 {
		return nil
	}

	hScale := 1.0
	vScale := 1.0
	if width > 0 {
		hScale = float64(width) / float64(vImg.Width())
	}
	if height > 0 {
		vScale = float64(height) / float64(vImg.Height())
	}

	scale := hScale
	if height > 0 && width > 0 {
		if vScale < hScale {
			scale = vScale
		}
	} else if height > 0 {
		scale = vScale
	}

	return vImg.Resize(scale, vips.KernelLanczos3)
}

func resizeOutside(vImg *vips.ImageRef, width, height, origWidth, origHeight int) error {
	if width == 0 && height == 0 {
		return nil
	}

	hScale := 1.0
	vScale := 1.0
	if width > 0 {
		hScale = float64(width) / float64(origWidth)
	}
	if height > 0 {
		vScale = float64(height) / float64(origHeight)
	}

	scale := hScale
	if height > 0 && width > 0 {
		if vScale > hScale {
			scale = vScale
		}
	} else if height > 0 {
		scale = vScale
	}

	return vImg.Resize(scale, vips.KernelLanczos3)
}

func parseInterest(position string) vips.Interesting {
	switch strings.ToLower(position) {
	case "centre", "center":
		return vips.InterestingCentre
	case "entropy":
		return vips.InterestingEntropy
	case "attention":
		return vips.InterestingAttention
	default:
		return vips.InterestingCentre
	}
}

func (img *Image) applyOperations(vImg *vips.ImageRef, p QueryParams) error {
	if p.Blur > 0 && p.Blur <= 100 {
		if err := vImg.GaussianBlur(p.Blur); err != nil {
			return fmt.Errorf("blur: %w", err)
		}
	}

	if p.Sharpen > 1 && p.Sharpen <= 100 {
		if err := vImg.Sharpen(p.Sharpen, 1.0, 2.0); err != nil {
			return fmt.Errorf("sharpen: %w", err)
		}
	}

	if p.Flip {
		if err := vImg.Flip(vips.DirectionVertical); err != nil {
			return fmt.Errorf("flip: %w", err)
		}
	}

	if p.Flop {
		if err := vImg.Flip(vips.DirectionHorizontal); err != nil {
			return fmt.Errorf("flop: %w", err)
		}
	}

	if p.Rotate != 0 {
		// govips Rotate takes a vips.Angle for 90-degree increments
		switch p.Rotate {
		case 90:
			if err := vImg.Rotate(vips.Angle90); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
		case 180:
			if err := vImg.Rotate(vips.Angle180); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
		case 270:
			if err := vImg.Rotate(vips.Angle270); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
		default:
			// For arbitrary angles, use Similarity with angle
			if err := vImg.Similarity(1.0, float64(p.Rotate), nil, 0, 0, 0, 0); err != nil {
				return fmt.Errorf("rotate arbitrary: %w", err)
			}
		}
	}

	return nil
}

func (img *Image) convertFormat(vImg *vips.ImageRef, p QueryParams, accept string) error {
	cfg := &config.Cfg

	quality := p.Quality
	if quality == 0 {
		quality = cfg.Quality
	}

	// Get image dimensions for megapixel check
	w := vImg.Width()
	h := vImg.Height()
	areaMp := float64(w*h) / 1000000.0

	logger.Debug("image dimensions", "width", w, "height", h, "areaMp", areaMp)

	// Check requested dimensions too
	reqAreaExceeds := false
	if p.Width > 0 && p.Height > 0 {
		reqAreaMp := float64(p.Width*p.Height) / 1000000.0
		reqAreaExceeds = reqAreaMp > cfg.AVIFMaxMP
	}

	maxAreaExceeds := areaMp > cfg.AVIFMaxMP
	if reqAreaExceeds {
		maxAreaExceeds = true
	}

	isGif := img.ContentType == "image/gif"

	if strings.Contains(accept, "image/avif") && !isGif && cfg.AVIF && !maxAreaExceeds {
		img.ContentType = "image/avif"
		ep := vips.NewAvifExportParams()
		ep.Quality = quality
		if p.Lossless != nil {
			ep.Lossless = *p.Lossless
		}
		if p.WebpEffort > 0 {
			ep.Speed = p.WebpEffort
		}
		buf, _, err := vImg.ExportAvif(ep)
		if err != nil {
			return fmt.Errorf("export avif: %w", err)
		}
		img.Data = buf
		return nil
	}

	if strings.Contains(accept, "image/webp") && cfg.WebP {
		img.ContentType = "image/webp"
		ep := vips.NewWebpExportParams()
		ep.Quality = quality
		if p.Lossless != nil {
			ep.Lossless = *p.Lossless
		}
		if p.WebpEffort > 0 {
			ep.ReductionEffort = p.WebpEffort
		}
		buf, _, err := vImg.ExportWebp(ep)
		if err != nil {
			return fmt.Errorf("export webp: %w", err)
		}
		img.Data = buf
		return nil
	}

	// Export in original format
	buf, _, err := vImg.ExportNative()
	if err != nil {
		return fmt.Errorf("export native: %w", err)
	}
	img.Data = buf
	return nil
}

// ResponseHeaders returns the HTTP response headers.
func (img *Image) ResponseHeaders() map[string]string {
	return map[string]string{
		"Content-Type":   img.ContentType,
		"Content-Length": strconv.Itoa(len(img.Data)),
		"Cache-Control":  "public, max-age=31536000",
		"Expires":        time.Now().Add(365 * 24 * time.Hour).UTC().Format(http.TimeFormat),
		"Last-Modified":  time.Now().UTC().Format(http.TimeFormat),
		"Pragma":         "public",
		"X-Powered-By":   "Image API",
	}
}

// Base64 returns the processed image data as a base64-encoded string.
func (img *Image) Base64() string {
	return base64.StdEncoding.EncodeToString(img.Data)
}

func extractDomain(rawURL string) string {
	// Extract domain from URL: "https://example.com/path" -> "example.com"
	parts := strings.SplitN(rawURL, "/", 4)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
