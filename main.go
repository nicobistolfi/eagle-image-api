// Command eagle-image-api is the AWS Lambda entry point for the Eagle image
// optimization and transformation service.
//
// Configuration comes entirely from environment variables; see the config
// package for the full list. The binary loads that configuration, starts
// libvips, and hands requests to the handler package.
package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/nicobistolfi/eagle-image-api/internal/config"
	"github.com/nicobistolfi/eagle-image-api/internal/handler"
	"github.com/nicobistolfi/eagle-image-api/internal/logger"
)

// setup loads configuration, installs the logger, and starts libvips. The
// returned function releases the libvips resources.
func setup() (shutdown func(), err error) {
	config.Load()
	logger.Init(config.Cfg.LogLevel)

	if err := vips.Startup(nil); err != nil {
		return nil, err
	}
	return vips.Shutdown, nil
}

func main() {
	shutdown, err := setup()
	if err != nil {
		panic("failed to start vips: " + err.Error())
	}
	defer shutdown()

	lambda.Start(handler.Handle)
}
