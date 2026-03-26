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

	if err := vips.Startup(nil); err != nil {
		panic("failed to start vips: " + err.Error())
	}
	defer vips.Shutdown()

	lambda.Start(handler.Handle)
}
