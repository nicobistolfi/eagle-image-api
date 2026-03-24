# Eagle Image API

[![Author](https://img.shields.io/badge/author-%40nicobistolfi-blue.svg)](https://github.com/nicobistolfi)
![License](https://img.shields.io/badge/license-MIT-green.svg)
[![Postman](https://img.shields.io/badge/Postman-Run%20in%20Postman-orange)](https://god.gw.postman.com/run-collection/22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8?action=collection%2Ffork&source=rip_markdown&collection-url=entityId%3D22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8%26entityType%3Dcollection%26workspaceId%3Dfa758360-633e-4980-857e-9500b71c1d81)

Free and open source Image Optimization & Transformation API built in Go using [govips](https://github.com/davidbyttow/govips) (libvips) and [Serverless](https://serverless.com/). Ready to be deployed on AWS Lambda and CloudFront.

## Table of contents

- [Getting started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Setting up .env file](#setting-up-env-file)
  - [Run locally](#run-locally)
  - [Deploy to AWS](#deploy-to-aws)
  - [Run in Postman](#run-in-postman)
- [Usage](#usage)
  - [Usage examples](#usage-examples)

## Getting started

### Prerequisites

- Go 1.22+
- libvips 8.10+ (`brew install vips` on macOS, `apt install libvips-dev` on Ubuntu)
- Docker (for deployment)
- Serverless Framework (`npm install -g serverless`)

### Setting up .env file

First copy and rename the `.env.example` file.

```
cp .env.example .env
```

Then change the values to your needs. Use the following table to understand what each variable does.

| Variable          | Accepted values                                    | Comments                                                 |
| ----------------- | -------------------------------------------------- | -------------------------------------------------------- |
| ENVIRONMENT       | production / development                           |                                                          |
| API_ENDPOINT      | /api/v1/image                                      | the path to access the api                               |
| PORT              | 3000                                               | The port you want it to run locally.                     |
| QUALITY           | 90                                                 | Suggested between 70 and 90 depending on the use case    |
| FIT               | outside                                            | Default resize fit mode                                  |
| LOG_LEVEL         | error / warn / info / debug                        |                                                          |
| ORIGIN_WHITELIST  | yourorigin.com,other.com                           | Use * to allow all origins                               |
| REDIRECT_ON_ERROR | false                                              | Will redirect to original image on error                 |
| WEBP              | true/false                                         | Enables webp format if accept header includes image/webp |
| AVIF              | true/false                                         | Enables avif format if accept header includes image/avif |
| AVIF_MAX_MP       | Number                                             | Maximum megapixels for avif format on output.            |

### Run locally

```bash
# Build
make build

# Run tests
make test
```

### Deploy to AWS

The API is deployed as a Docker container on AWS Lambda via Serverless Framework.

```bash
# Deploy to dev
make deploy-dev

# Deploy to production
make deploy-prod
```

### Run in Postman

To run and test the API you can use the following Postman collection

[<img src="https://run.pstmn.io/button.svg" alt="Run In Postman" style="width: 128px; height: 32px;">](https://god.gw.postman.com/run-collection/22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8?action=collection%2Ffork&source=rip_markdown&collection-url=entityId%3D22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8%26entityType%3Dcollection%26workspaceId%3Dfa758360-633e-4980-857e-9500b71c1d81)

## Usage

The API uses query params to transform the images.

| Query param  | Description                                                                                              | Values                                                                   |
| ------------ | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| url          | URL that points to the original location of the image (URL-encoded)                                      | String (required)                                                        |
| width        | Width                                                                                                    | Number                                                                   |
| height       | Height                                                                                                   | Number                                                                   |
| fit          | When both a width and height are provided, the possible methods by which the image should fit these are: | cover / contain / fill / inside / outside                                |
| position     | When using a fit of cover or contain, the default position is centre. Other options are:                 | top, right top, right, right bottom, bottom, left bottom, left, left top |
| quality      | Quality                                                                                                  | Number (0-100)                                                           |
| lossless     | Use lossless compression mode                                                                            | Boolean                                                                  |
| blur         | Gaussian blur                                                                                            | Number (0-100)                                                           |
| sharpen      | Sharpen                                                                                                  | Number (1-100)                                                           |
| flip         | Flip vertically                                                                                          | Boolean                                                                  |
| flop         | Flip horizontally                                                                                        | Boolean                                                                  |
| rotate       | Rotate by angle                                                                                          | Number (degrees)                                                         |
| webpEffort   | WebP compression effort                                                                                  | Number (0-6)                                                             |
| alphaQuality | Quality of alpha layer                                                                                   | Number (0-100)                                                           |
| loop         | GIF number of animation iterations, use 0 for infinite                                                   | Number                                                                   |
| delay        | GIF delay between frames (in milliseconds)                                                               | Number                                                                   |

### Usage examples

Replace `API_ENDPOINT` with your deployed URL.

#### Resize

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&url={{IMAGE_URL}}
```

#### Resize and crop with fit cover

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&url={{IMAGE_URL}}
```

#### Resize and crop with fit contain

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=contain&url={{IMAGE_URL}}
```

#### Resize and crop with position top

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&url={{IMAGE_URL}}
```

#### Resize and crop with position and quality

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&quality=10&url={{IMAGE_URL}}
```

#### Lossless compression

```
{{API_ENDPOINT}}/api/v1/image?quality=80&lossless=true&url={{IMAGE_URL}}
```
