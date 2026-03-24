package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/zantez/image-api/internal/config"
	"github.com/zantez/image-api/internal/handler"
	"github.com/zantez/image-api/internal/logger"
)

func main() {
	config.Load()
	logger.Init(config.Cfg.LogLevel)

	vips.Startup(nil)
	defer vips.Shutdown()

	lambda.Start(handler.Handle)
}
