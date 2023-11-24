# Image API

[![Author](https://img.shields.io/badge/author-%40nicobistolfi-blue.svg)](https://github.com/nicobistolfi)

Free and open source Image Optimization & Transformation API build on Typescript using [Sharp](https://sharp.pixelplumbing.com/) and [Serverless](https://serverless.com/) ready to be deployed on AWS Lambda and Cloudfront.

## Getting started

### Setting up .env file

```
cp .env.example .env
```

Change the values to your needs and use the following table to understand what each variable does.

| Variable          | Accepted values                                    | Comments                                              |
| ----------------- | -------------------------------------------------- | ----------------------------------------------------- |
| ENVIRONMENT       | production / development                           |                                                       |
| API_ENDPOINT      | /api/v1/image                                      | the path to access the api                            |
| PORT              | 3000                                               | The port you want it to run locally.                  |
| LOSELESS          | false                                              | Use looseless compression                             |
| QUALITY           | 90                                                 | Suggested between 70 and 90 depending on the use case |
| LOG_LEVEL         | error warn / info / http / verbose / debug / silly |                                                       |
| ORIGIN_WHITELIST  | ['yourorigin.com']                                 | remove env to allow all origins                       |
| REDIRECT_ON_ERROR | false                                              | Will redirect to original image on error              |

### Run locally

```
npm install
serverless offline
```

### Deploy to AWS

```
npm install
serverless deploy --stage {your_stage}
```

## Usage

The API uses query params to transform the images. The following table shows the available options. More information on how the transformations work can be found on [Sharp's Documentation](https://sharp.pixelplumbing.com/).

| Query param  | Description                                                                                              | Values                                                                   |
| ------------ | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| src          | Image url encoded with base64                                                                            | String                                                                   |
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

#### Resize and crop

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&url={{IMAGE_URL}}
```

#### Resize and crop with position

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&url={{IMAGE_URL}}
```

#### Resize and crop with position and quality

```
{{API_ENDPOINT}}/api/v1/image?width=400&height=400&fit=cover&position=top&quality=80&url={{IMAGE_URL}}
```

#### Lossless compression

```
{{API_ENDPOINT}}/api/v1/image?quality=80&lossless=true&url={{IMAGE_URL}}
```
