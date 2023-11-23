import { Response, Request, Router } from 'express';
import { Image } from '../lib/image';
import { info, debug, error } from '../utils/logger';

import { ENVIRONMENT, REDIRECT_ON_ERROR, API_ENDPOINT } from '../utils/env';

const route = Router();

route.get('/health', async (req: Request, res: Response) => {
  info('GET %s 🦅', req.originalUrl);
  res.send('🦅');
});

route.get(API_ENDPOINT, async (req: Request, res: Response) => {
  info('GET %s', req.originalUrl);
  debug('GET %s %s', req.url, req.query);
  try {
    const image = new Image(req.query.url as string);
    const isImage = await image.isImage();
    if (!isImage) {
      res.status(404).send('Not found');
      return;
    }
    await image.loadImage();
    await image.processRequest(req);
    // image.setResponseHeaders(res);
    const body = await image.buffer();
    // console.log(body?.toString('base64'));
    res.json({
      body: body?.toString('base64'),
      isBase64Encoded: true,
    });
    // res.send(body);
    // res.send({
    //   body: body?.toString('base64'),
    //   isBase64Encoded: true,
    // });

    res.end();
  } catch (err: any) {
    error('GET %s \n\n%s', req.url, err);
    if (REDIRECT_ON_ERROR) {
      res.redirect(req.query.url as string);
    } else if (ENVIRONMENT === 'development') {
      res.status(err.response ? err.response.status : 500).send(err.message);
    } else {
      res
        .status(err.response ? err.response.status : 500)
        .send('Internal server error');
    }
  }
});

export default route;
