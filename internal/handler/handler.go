// Package handler routes API Gateway requests to the image pipeline.
//
// Three paths are served: /health for liveness checks, the configured
// [config.Config.APIEndpoint] for transformations, and everything else as a
// 404. Errors are reported as 400 responses, or as a 302 redirect back to the
// source image when REDIRECT_ON_ERROR is enabled, so a broken transformation
// degrades to the original asset instead of a broken image on the page.
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/nicobistolfi/eagle-image-api/internal/config"
	"github.com/nicobistolfi/eagle-image-api/internal/image"
	"github.com/nicobistolfi/eagle-image-api/internal/logger"
)

// Handle is the AWS Lambda entry point. It accepts API Gateway proxy events
// and always returns a response with a nil error: failures are communicated
// through the HTTP status code so API Gateway does not surface a 502.
func Handle(_ context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger.Debug("request", "method", event.HTTPMethod, "path", event.Path)

	if event.HTTPMethod != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "Method Not Allowed"), nil
	}

	return handleGet(event), nil
}

// handleGet dispatches a GET by path.
func handleGet(event events.APIGatewayProxyRequest) events.APIGatewayProxyResponse {
	switch event.Path {
	case "/health":
		logger.Debug("health endpoint")
		return textResponse(http.StatusOK, "\U0001F985") // 🦅
	case config.Cfg.APIEndpoint:
		logger.Debug("api endpoint")
		return processImage(event)
	default:
		logger.Debug("unknown endpoint", "path", event.Path)
		return textResponse(http.StatusNotFound, "Not Found")
	}
}

// processImage fetches, transforms, and encodes the requested image.
func processImage(event events.APIGatewayProxyRequest) events.APIGatewayProxyResponse {
	imageURL := event.QueryStringParameters["url"]
	if imageURL == "" {
		return textResponse(http.StatusBadRequest, "Missing required parameter: url")
	}

	img := image.New(imageURL)

	// A HEAD request first avoids downloading a whole HTML error page when
	// the URL does not actually point at an image.
	isImg, err := img.IsImage()
	if err != nil {
		return handleError(event, err)
	}
	if !isImg {
		logger.Debug("HEAD check: not an image", "url", imageURL)
		return textResponse(http.StatusNotFound, "Not Found")
	}

	if err := img.Load(); err != nil {
		return handleError(event, err)
	}

	params := image.ParseQueryParams(event.QueryStringParameters)
	if err := img.Process(params, findAcceptHeader(event.Headers)); err != nil {
		return handleError(event, err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode:      http.StatusOK,
		Headers:         img.ResponseHeaders(),
		Body:            img.Base64(),
		IsBase64Encoded: true,
	}
}

// findAcceptHeader returns the Accept header value regardless of the casing
// API Gateway happens to deliver.
func findAcceptHeader(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "accept") {
			return v
		}
	}
	return ""
}

// handleError turns a processing failure into a response: a 400 by default,
// or a redirect to the source image when RedirectOnError is enabled.
func handleError(event events.APIGatewayProxyRequest, err error) events.APIGatewayProxyResponse {
	logger.Error("request failed", "path", event.Path, "error", err.Error())

	if config.Cfg.RedirectOnError {
		if url := event.QueryStringParameters["url"]; url != "" {
			logger.Error("redirecting to original", "url", url)
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusFound,
				Headers:    map[string]string{"Location": url},
			}
		}
	}

	return textResponse(http.StatusBadRequest, err.Error())
}

// textResponse builds a plain response carrying only a status and body.
func textResponse(status int, body string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: status, Body: body}
}
