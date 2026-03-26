package handler

import (
	"context"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/zantez/image-api/internal/config"
	"github.com/zantez/image-api/internal/image"
	"github.com/zantez/image-api/internal/logger"
)

// Handle is the AWS Lambda handler function.
func Handle(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger.Debug("request", "method", event.HTTPMethod, "path", event.Path)

	switch event.HTTPMethod {
	case "GET":
		return handleGet(event)
	default:
		return events.APIGatewayProxyResponse{
			StatusCode: 405,
			Body:       "Method Not Allowed",
		}, nil
	}
}

func handleGet(event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch event.Path {
	case "/health":
		logger.Debug("health endpoint")
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Body:       "\U0001F985", // 🦅
		}, nil
	case config.Cfg.APIEndpoint:
		logger.Debug("api endpoint")
		return processImage(event)
	default:
		logger.Debug("unknown endpoint", "path", event.Path)
		return events.APIGatewayProxyResponse{
			StatusCode: 404,
			Body:       "Not Found",
		}, nil
	}
}

func processImage(event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	imageURL := event.QueryStringParameters["url"]
	if imageURL == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Body:       "Missing required parameter: url",
		}, nil
	}

	img := image.New(imageURL)

	isImg, err := img.IsImage()
	if err != nil {
		return handleError(event, err)
	}
	if !isImg {
		logger.Debug("HEAD check: not an image", "url", imageURL)
		return events.APIGatewayProxyResponse{
			StatusCode: 404,
			Body:       "Not Found",
		}, nil
	}

	if err := img.Load(); err != nil {
		return handleError(event, err)
	}

	params := image.ParseQueryParams(event.QueryStringParameters)
	acceptHeader := findAcceptHeader(event.Headers)

	if err := img.Process(params, acceptHeader); err != nil {
		return handleError(event, err)
	}

	headers := img.ResponseHeaders()
	return events.APIGatewayProxyResponse{
		StatusCode:      200,
		Headers:         headers,
		Body:            img.Base64(),
		IsBase64Encoded: true,
	}, nil
}

func findAcceptHeader(headers map[string]string) string {
	for k, v := range headers {
		if strings.ToLower(k) == "accept" {
			return v
		}
	}
	return ""
}

func handleError(event events.APIGatewayProxyRequest, err error) (events.APIGatewayProxyResponse, error) {
	logger.Error("request failed", "path", event.Path, "error", err.Error())

	if config.Cfg.RedirectOnError {
		if url := event.QueryStringParameters["url"]; url != "" {
			logger.Error("redirecting to original", "url", url)
			return events.APIGatewayProxyResponse{
				StatusCode: 302,
				Headers: map[string]string{
					"Location": url,
				},
			}, nil
		}
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 400,
		Body:       err.Error(),
	}, nil
}
