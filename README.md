# Eagle Image API

[![Author](https://img.shields.io/badge/author-%40nicobistolfi-blue.svg)](https://github.com/nicobistolfi)
![License](https://img.shields.io/badge/license-MIT-green.svg)
[![Postman](https://img.shields.io/badge/Postman-Run%20in%20Postman-orange)](https://god.gw.postman.com/run-collection/22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8?action=collection%2Ffork&source=rip_markdown&collection-url=entityId%3D22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8%26entityType%3Dcollection%26workspaceId%3Dfa758360-633e-4980-857e-9500b71c1d81)

Free and open source Image Optimization & Transformation API build on Typescript using [Sharp](https://sharp.pixelplumbing.com/) and [Serverless](https://serverless.com/). Ready to be deployed on AWS Lambda and Cloudfront.

## How to deploy to AWS and Run
[![How to deploy to AWS and Run - Watch Video](https://cdn.loom.com/sessions/thumbnails/97692855cf3947eabd45dd5e90a548ec-c333bef1399a2164-full-play.gif)](https://www.loom.com/share/97692855cf3947eabd45dd5e90a548ec)

## Table of contents

- [Getting started](#getting-started)
  - [Setting up .env file](#setting-up-env-file)
  - [Run locally](#run-locally)
  - [Deploy to AWS](#deploy-to-aws)
  - [Run in Postman](#run-in-postman)
- [Usage](#usage)
  - [Usage examples](#usage-examples)
    - [Resize](#resize)
    - [Resize and crop with fit cover](#resize-and-crop-with-fit-cover)
    - [Resize and crop with fit contain](#resize-and-crop-with-fit-contain)
    - [Resize and crop with position top](#resize-and-crop-with-position-top)
    - [Resize and crop with position and quality](#resize-and-crop-with-position-and-quality)
    - [Lossless compression](#lossless-compression)

## Getting started

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
| LOSELESS          | false                                              | Use looseless compression                                |
| QUALITY           | 90                                                 | Suggested between 70 and 90 depending on the use case    |
| LOG_LEVEL         | error warn / info / http / verbose / debug / silly |                                                          |
| ORIGIN_WHITELIST  | ['yourorigin.com']                                 | remove env to allow all origins                          |
| REDIRECT_ON_ERROR | false                                              | Will redirect to original image on error                 |
| WEBP              | true/false                                         | Enables webp format if accept header includes image/webp |
| AVIF              | true/false                                         | Enables avif format if accept header includes image/avif |
| AVIF_MAX_MP       | Number                                             | Maximum megapixels for avif format on output.            |

### Run locally

```
npm install
serverless offline
```

By default the API will run on port 3000. You can change it on the `.env` file by setting the `PORT` variable.

You can try it by accessing the following url on your browser:

```
http://localhost:3000/dev/api/v1/image?url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg
```

The `url` query param is required and should be a valid url encoded string. In this case for testing purposes we are using an image hosted on S3. The original image URL is:

```
https://eagle-image-test.s3.us-west-1.amazonaws.com/public/eagle.jpeg
```

<img src="https://eagle-image-test.s3.us-west-1.amazonaws.com/public/eagle.jpeg" width="400px" alt="Photo by Jack Seeds on Unsplash" />

Photo by [Jack Seeds](https://unsplash.com/@jackseeds?utm_content=creditCopyText&utm_medium=referral&utm_source=unsplash) on [Unsplash](https://unsplash.com/photos/black-hawk-on-tree-lV5Y2116NZg?utm_content=creditCopyText&utm_medium=referral&utm_source=unsplash)

### Deploy to AWS

```
npm install
serverless deploy --stage {your_stage}
```

### Run in Postman

To run and test the API you can use the following Postman collection

[<img src="https://run.pstmn.io/button.svg" alt="Run In Postman" style="width: 128px; height: 32px;">](https://god.gw.postman.com/run-collection/22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8?action=collection%2Ffork&source=rip_markdown&collection-url=entityId%3D22482580-83154c76-0cda-4385-848b-bdaa0bef7fb8%26entityType%3Dcollection%26workspaceId%3Dfa758360-633e-4980-857e-9500b71c1d81)

## Usage

The API uses query params to transform the images. The following table shows the available options. More information on how the transformations work can be found on [Sharp's Documentation](https://sharp.pixelplumbing.com/).

| Query param  | Description                                                                                              | Values                                                                   |
| ------------ | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| src          | Url that points to the original location of the image in the form of a url encoded string                | String                                                                   |
| width        | Width                                                                                                    | Number                                                                   |
| height       | Height                                                                                                   | Number                                                                   |
| fit          | When both a width and height are provided, the possible methods by which the image should fit these are: | cover / contain / fill / inside / outside                                |
| position     | When using a fit of cover or contain, the default position is centre. Other options are:                 | top, right top, right, right bottom, bottom, left bottom, left, left top |
| quality      | Quality                                                                                                  | Number (0-100)                                                           |
| lossless     | use lossless compression mode                                                                            | Boolean                                                                  |
| effort       | CPU effort, between 0 (fastest) and 6 (slowest)                                                          | Number (0-6)                                                             |
| alphaQuality | quality of alpha layer, integer 0-100                                                                    | Number (0-100)                                                           |
| loop         | GIF number of animation iterations, use 0 for infinite animation                                         | Number                                                                   |
| delay        | GIF delay(s) between animation frames (in milliseconds)                                                  | Number                                                                   |

### Usage examples

You can use the following examples to understand how to use it. Remember to replace the `API_ENDPOINT` with the one you set on your `.env` file.

#### Resize

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)

#### Resize and crop with fit cover

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)

#### Resize and crop with fit contain

This will deform the image to fit the size

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=contain&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?fit=contain&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?fit=contain&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)

#### Resize and crop with position top

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?position=top&fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?position=top&fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)

#### Resize and crop with position and quality

For the purpose of this test we set quality=10 just to test it, normally you should use quality between 70-90

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&quality=10&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?quality=10&position=top&fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?quality=10&position=top&fit=cover&width=400&height=400&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)

#### Lossless compression

```
{{API_ENDPOINT}}/api/v1/image?quality=80&lossless=true&url={{IMAGE_URL}}
```

<img src="https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?width=400&quality=80&loseless=true&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg" width="200px" alt="Resized image" />

[View original api response here](https://d3kbkk2pyl9ybg.cloudfront.net/api/v1/image?width=400&quality=80&loseless=true&url=https%3A%2F%2Feagle-image-test.s3.us-west-1.amazonaws.com%2Fpublic%2Feagle.jpeg)
