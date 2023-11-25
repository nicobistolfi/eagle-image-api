import axios from 'axios';
import { Request, Response } from 'express';
import sharp, { Sharp } from 'sharp';
import {
  QUALITY,
  FIT,
  ORIGIN_WHITELIST,
  WEBP,
  AVIF,
  AVIF_MAX_MP,
} from '../utils/env';
import { toBoolean } from '../utils/helpers';
import { debug, silly } from '../utils/logger';

const defaultHeaders = {
  'User-Agent':
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36',
  Accept: '*/*',
  'Accept-Encoding': 'gzip, deflate, br',
  Connection: 'keep-alive',
  'Upgrade-Insecure-Requests': '1',
};

class Image {
  url: string;
  // eslint-disable-next-line lines-between-class-members
  type: string | undefined;
  data: Buffer | undefined;
  width: number | undefined;
  height: number | undefined;
  s: Sharp | undefined;

  constructor(url: string) {
    this.url = url;
  }

  async isImage(): Promise<boolean> {
    const req = await axios.head(this.url, {
      headers: defaultHeaders,
    });
    return (req.headers['content-type'] as string).includes('image');
  }

  async loadImage(): Promise<Image> {
    // check if url is from whitelisted domain
    if (
      ORIGIN_WHITELIST !== '*' &&
      !ORIGIN_WHITELIST.includes(this.url.split('/')[2])
    ) {
      throw new Error('Origin not allowed');
    }
    const res = await axios.get(this.url, {
      headers: defaultHeaders,
      responseType: 'arraybuffer',
    });
    this.type = res.headers['content-type'] as string;
    this.data = Buffer.from(res.data, 'base64');
    this.s = sharp(this.data);
    return this;
  }

  async processRESTEvent(event: any): Promise<Image> {
    const q: any = event.queryStringParameters;
    const h: [string, any][] = Object.entries(event.headers);
    silly('q: %o', q);
    silly('h: %o', h);

    const accept = h[h.findIndex((e) => e[0].toLowerCase() === 'accept')][1];

    this.resizeByQuery(q);
    this.performOperationsByQuery(q);
    await this.convertFormatByAccept(q, accept);
    this.data = await this.s?.toBuffer();
    return this;
  }

  resizeByQuery(q: any): void {
    this.s?.resize(
      Number(q.width as string) || undefined,
      Number(q.height as string) || undefined,
      {
        fit: q.fit || (FIT as string),
        position: (q.position as string) || undefined,
        kernel: q.kernel || undefined,
        background: (q.background as string) || undefined,
        // withoutEnlargement: q.withoutEnlargement as boolean || undefined,
        // withoutReduction: q.withoutReduction as boolean || undefined,
        // fastShrinkOnLoad: q.fastShrinkOnLoad as boolean || undefined,
      },
    );
    this.width = q.width ? Number(q.width) : undefined;
    this.height = q.height ? Number(q.height) : undefined;
  }

  async convertFormatByAccept(q: any, accept: string = ''): Promise<void> {
    // return compatible format
    debug('header.accept: %s', accept);

    // check if image area is bigger than 5 megapixels
    // don't convert to AVIF if image area is bigger than 5 megapixels
    const metadata = await this.s?.metadata();
    this.width = metadata?.width;
    this.height = metadata?.height;
    // eslint-disable-next-line operator-linebreak
    debug('width: %s, height: %s', this.width, this.height);
    // get width and height from sharp object

    const imageMaxAreaSizeExceeds =
      this.width && this.height
        ? (this.width * this.height) / 1000000 > AVIF_MAX_MP
        : false;

    const maxAreaSizeExceeds =
      q.width && q.height
        ? (Number(q.width) * Number(q.height)) / 1000000 > AVIF_MAX_MP
        : imageMaxAreaSizeExceeds;

    if (
      accept.includes('image/avif') &&
      this.type !== 'image/gif' &&
      AVIF &&
      !maxAreaSizeExceeds
    ) {
      this.type = 'image/avif';
      this.s?.avif({
        quality: (Number(q.quality) as number) || QUALITY,
        lossless: q.lossless !== undefined ? toBoolean(q.lossless) : undefined,
        effort: (Number(q.webpEffort) as number) || undefined,
      });
    } else if (accept.includes('image/webp') && WEBP) {
      this.type = 'image/webp';
      this.s?.webp({
        quality: (Number(q.quality) as number) || QUALITY,
        lossless: q.lossless !== undefined ? toBoolean(q.lossless) : undefined,
        alphaQuality: (Number(q.alphaQuality) as number) || undefined,
        effort: (Number(q.webpEffort) as number) || undefined,
        loop: (Number(q.loop) as number) || undefined,
        delay: (Number(q.delay) as number) || undefined,
      });
    }
  }

  // eslint-disable-next-line class-methods-use-this
  performOperationsByQuery(q: any): void {
    if (q.blur && Number(q.blur) > 0 && Number(q.blur) <= 100) {
      this.s?.blur(Number(q.blur) as number);
    }
    if (q.sharpen && Number(q.sharpen) > 1 && Number(q.sharpen) <= 100) {
      this.s?.sharpen(Number(q.sharpen) as number);
    }
    if (q.flip) {
      this.s?.flip();
    }
    if (q.flop) {
      this.s?.flop();
    }
    if (q.rotate) {
      this.s?.rotate(Number(q.rotate) as number, {
        background: (q.background as string) || undefined,
      });
    }
  }

  getResponseHeaders(): any {
    const headers = {
      'Content-Type': this.type as string,
      'Content-Length': this.data?.length || 0,
      'Cache-Control': 'public, max-age=31536000',
      Expires: new Date(Date.now() + 31536000).toUTCString(),
      'Last-Modified': new Date().toUTCString(),
      Pragma: 'public',
      'X-Powered-By': 'Image API',
    };

    return headers;
  }

  async buffer(): Promise<Buffer | undefined> {
    // info(await ((await this.s?.toBuffer()) || '').toString('base64'));
    return this.s?.toBuffer();
  }
}

export default Image;
export { Image };
