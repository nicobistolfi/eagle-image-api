import { API_ENDPOINT } from './utils/env';
import Image from './lib/image';
import { debug, error, silly } from './utils/logger';

const err404 = {
  statusCode: 404,
  body: 'Not Found',
};

const health = () => ({
  statusCode: 200,
  body: '🦅',
});

const api = async (event: any) => {
  try {
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
  } catch (err: any) {
    error('GET %s \n\n%s', event.path, err);
    return {
      statusCode: 400,
      body: err.message,
    };
  }
};

const get = async (event: any) => {
  try {
    switch (event.path) {
      case '/health':
        silly('health endpoint');
        return health();
      case API_ENDPOINT:
        silly('api endpoint');
        return api(event);
      default:
        silly('unknown endpoint');
        return {
          statusCode: 404,
          body: 'Not Found',
        };
    }
  } catch (err: any) {
    error('GET %s \n\n%s', event.path, err);
    return {
      statusCode: 400,
      body: err.message,
    };
  }
};

const handler = async (event: any, context: any) => {
  debug('GET %s', event.path);
  try {
    switch (event.httpMethod) {
      case 'GET':
        return get(event);
      default:
        return {
          statusCode: 405,
          body: 'Method Not Allowed',
        };
    }
  } catch (err: any) {
    error('GET %s \n\n%s', event.path, err);
    return {
      statusCode: 400,
      body: err.message,
    };
  }
};

export { handler };

export default handler;
