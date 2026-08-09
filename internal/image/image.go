// Package image fetches remote images and applies the requested
// transformations using libvips through govips.
//
// A request follows a fixed pipeline: [Image.IsImage] cheaply rejects URLs
// that are not images, [Image.Load] downloads the bytes subject to the origin
// allowlist, and [Image.Process] resizes, applies operations, and encodes the
// result. Output format is negotiated from the client's Accept header, with
// AVIF preferred over WebP and a megapixel ceiling that keeps slow AVIF
// encodes from exhausting the Lambda timeout.
//
// Callers must have started libvips with vips.Startup before using this
// package.
package image

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/nicobistolfi/eagle-image-api/internal/config"
	"github.com/nicobistolfi/eagle-image-api/internal/logger"
)

// ErrOriginNotAllowed is returned by [Image.Load] when the image URL's domain
// is not in the configured origin allowlist.
//
// The message is capitalised because it is served verbatim as the HTTP
// response body, which is part of the API's contract with existing clients.
//
//lint:ignore ST1005 the text is a client-facing response body
var ErrOriginNotAllowed = errors.New("Origin not allowed") //nolint:staticcheck // client-facing message

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

// ParseQueryParams converts a map of request parameters to [QueryParams].
//
// Malformed numeric values are left at their zero value rather than rejected,
// so a typo in one parameter degrades that single transformation instead of
// failing the whole request.
func ParseQueryParams(m map[string]string) QueryParams {
	p := QueryParams{
		URL:          m["url"],
		Fit:          m["fit"],
		Position:     m["position"],
		Background:   m["background"],
		Kernel:       m["kernel"],
		Width:        intParam(m, "width"),
		Height:       intParam(m, "height"),
		Quality:      intParam(m, "quality"),
		Rotate:       intParam(m, "rotate"),
		AlphaQuality: intParam(m, "alphaQuality"),
		Loop:         intParam(m, "loop"),
		Delay:        intParam(m, "delay"),
		WebpEffort:   intParam(m, "webpEffort"),
		Blur:         floatParam(m, "blur"),
		Sharpen:      floatParam(m, "sharpen"),
		Lossless:     boolPtrParam(m, "lossless"),
	}

	// flip and flop are flags: presence alone turns them on, so `?flip` works
	// as well as `?flip=1`.
	_, p.Flip = m["flip"]
	_, p.Flop = m["flop"]

	return p
}

// intParam reads an integer parameter, returning 0 when it is absent, empty,
// or unparseable.
func intParam(m map[string]string, key string) int {
	n, _ := strconv.Atoi(m[key])
	return n
}

// floatParam reads a float parameter, returning 0 when it is absent, empty,
// or unparseable.
func floatParam(m map[string]string, key string) float64 {
	f, _ := strconv.ParseFloat(m[key], 64)
	return f
}

// boolPtrParam reads an optional boolean. It returns nil when the parameter is
// absent, which keeps "not specified" distinct from "explicitly false" so the
// encoder default is preserved.
func boolPtrParam(m map[string]string, key string) *bool {
	v, ok := m[key]
	if !ok {
		return nil
	}
	b := strings.EqualFold(v, "true") || v == "1"
	return &b
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
		if !slices.Contains(cfg.Origins, domain) {
			return ErrOriginNotAllowed
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

// resize scales the image according to the requested fit mode, then records
// the resulting dimensions on img.
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

	if err := applyFit(vImg, fit, p, origWidth, origHeight); err != nil {
		return err
	}

	img.Width = vImg.Width()
	img.Height = vImg.Height()
	return nil
}

// applyFit dispatches to the resize strategy named by fit. An unrecognised
// mode falls back to "contain", which is the least surprising behaviour: the
// whole image stays visible within the requested box.
func applyFit(vImg *vips.ImageRef, fit string, p QueryParams, origWidth, origHeight int) error {
	switch fit {
	case "cover":
		// Scale to cover both dimensions, then crop away the overflow.
		return resizeCover(vImg, p.Width, p.Height, p.Position)
	case "fill":
		// Stretch to the exact dimensions, ignoring the aspect ratio.
		return resizeFill(vImg, p.Width, p.Height, origWidth, origHeight)
	case "outside":
		// Scale so the image covers the box on its smallest side.
		return resizeOutside(vImg, p.Width, p.Height, origWidth, origHeight)
	case "contain", "inside":
		return resizeContain(vImg, p.Width, p.Height)
	default:
		return resizeContain(vImg, p.Width, p.Height)
	}
}

// resizeFill stretches the image to exactly width x height. Both dimensions
// are required; with only one there is no second scale factor to apply, so the
// image is left untouched.
func resizeFill(vImg *vips.ImageRef, width, height, origWidth, origHeight int) error {
	if width <= 0 || height <= 0 {
		return nil
	}

	hScale := float64(width) / float64(origWidth)
	vScale := float64(height) / float64(origHeight)
	return vImg.ResizeWithVScale(hScale, vScale, vips.KernelLanczos3)
}

func resizeCover(vImg *vips.ImageRef, width, height int, position string) error {
	if width == 0 || height == 0 {
		// If only one dimension, just resize that dimension
		var scale float64
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
		if err := rotate(vImg, p.Rotate); err != nil {
			return err
		}
	}

	return nil
}

// rotate turns the image by degrees. Right-angle turns use the cheap
// quadrant rotation; anything else goes through an affine transform.
func rotate(vImg *vips.ImageRef, degrees int) error {
	quadrants := map[int]vips.Angle{
		90:  vips.Angle90,
		180: vips.Angle180,
		270: vips.Angle270,
	}

	if angle, ok := quadrants[degrees]; ok {
		if err := vImg.Rotate(angle); err != nil {
			return fmt.Errorf("rotate: %w", err)
		}
		return nil
	}

	// For arbitrary angles, use Similarity. The background must not be nil:
	// govips dereferences it without a check, so passing nil crashes the
	// process. Transparent black matches what libvips uses to fill the
	// corners the rotation exposes.
	background := &vips.ColorRGBA{}
	if err := vImg.Similarity(1.0, float64(degrees), background, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("rotate arbitrary: %w", err)
	}
	return nil
}

// convertFormat encodes the processed image, negotiating AVIF or WebP from the
// client's Accept header and falling back to the source format.
func (img *Image) convertFormat(vImg *vips.ImageRef, p QueryParams, accept string) error {
	quality := p.Quality
	if quality == 0 {
		quality = config.Cfg.Quality
	}

	switch {
	case img.avifAllowed(vImg, p, accept):
		return img.exportAvif(vImg, p, quality)
	case strings.Contains(accept, "image/webp") && config.Cfg.WebP:
		return img.exportWebp(vImg, p, quality)
	default:
		return img.exportNative(vImg)
	}
}

// avifAllowed reports whether the result should be encoded as AVIF. AVIF is
// the best format on offer but also the slowest to encode, so it is limited to
// still images below a configurable megapixel ceiling — above that, encoding
// can outlast the Lambda timeout.
func (img *Image) avifAllowed(vImg *vips.ImageRef, p QueryParams, accept string) bool {
	if !strings.Contains(accept, "image/avif") || !config.Cfg.AVIF {
		return false
	}
	if img.ContentType == "image/gif" {
		return false
	}

	w, h := vImg.Width(), vImg.Height()
	areaMp := megapixels(w, h)
	logger.Debug("image dimensions", "width", w, "height", h, "areaMp", areaMp)

	if areaMp > config.Cfg.AVIFMaxMP {
		return false
	}
	// The requested size counts too: a small source upscaled past the ceiling
	// is just as expensive to encode.
	if p.Width > 0 && p.Height > 0 && megapixels(p.Width, p.Height) > config.Cfg.AVIFMaxMP {
		return false
	}

	return true
}

// megapixels returns the pixel area of width x height in megapixels.
func megapixels(width, height int) float64 {
	return float64(width*height) / 1_000_000.0
}

func (img *Image) exportAvif(vImg *vips.ImageRef, p QueryParams, quality int) error {
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

	img.ContentType = "image/avif"
	img.Data = buf
	return nil
}

func (img *Image) exportWebp(vImg *vips.ImageRef, p QueryParams, quality int) error {
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

	img.ContentType = "image/webp"
	img.Data = buf
	return nil
}

// exportNative re-encodes the image in the format it arrived in.
func (img *Image) exportNative(vImg *vips.ImageRef) error {
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
