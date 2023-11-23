import express, { Application } from 'express';
import * as awsServerlessExpress from 'aws-serverless-express';

import route from './routes/index';
import { API_ENDPOINT } from './utils/env';
import Image from './lib/image';
import { debug } from './utils/logger';

const app: Application = express();
const cors = require('cors');

app.use(cors());
app.use(route);

// const handler = serverless(app);

const err404 = {
  statusCode: 404,
  body: 'Not Found',
};

const health = () => ({
  statusCode: 200,
  body: '🦅',
});

const api = async (event: any) => {
  const imageUrl: string = event.queryStringParameters.url;
  const image: Image = new Image(imageUrl);
  const isImage: boolean = await image.isImage();
  if (!isImage) {
    debug(`HEAD ${imageUrl} is not image`);
    return err404;
  }
  await image.loadImage();
  await image.processRESTEvent(event);
  const body = await (await image.buffer())?.toString('base64');

  return {
    statusCode: 200,
    headers: image.getResponseHeaders(),
    body,
    isBase64Encoded: true,
  };
};

const get = async (event: any, context: any) => {
  switch (event.path) {
    case '/health':
      return health();
    case API_ENDPOINT:
      return api(event, context);
    default:
      return {
        statusCode: 404,
        body: 'Not Found',
      };
  }
};

const handler = async (event: any, context: any) => {
  switch (event.httpMethod) {
    case 'GET':
      return await get(event, context);
    default:
      return {
        statusCode: 405,
        body: 'Method Not Allowed',
      };
  }
};

export { handler };

export default handler;
