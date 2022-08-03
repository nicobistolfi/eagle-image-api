import axios from 'axios'
import { Request, Response } from 'express'
import sharp, { Sharp } from 'sharp'
import { QUALITY, FIT, ORIGIN_WHITELIST } from '../utils/env'
import { toBoolean } from '../utils/helpers'

const defaultHeaders = {
  'User-Agent':
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36',
  Accept: '*/*',
  'Accept-Encoding': 'gzip, deflate, br',
  Connection: 'keep-alive',
  'Upgrade-Insecure-Requests': '1',
}

class Image {
  url: string
  // eslint-disable-next-line lines-between-class-members
  type: string | undefined
  data: Buffer | undefined
  width: number | undefined
  height: number | undefined
  s: Sharp | undefined

  constructor(url: string) {
    this.url = url
  }

  async isImage(): Promise<boolean> {
    const req = await axios.head(this.url, {
      headers: defaultHeaders,
    })
    return (req.headers['content-type'] as string).includes('image')
  }

  async loadImage(): Promise<Image> {
    // check if url is from whitelisted domain
    if (ORIGIN_WHITELIST !== '*' && !ORIGIN_WHITELIST.includes(this.url.split('/')[2])) {
      throw new Error('Origin not allowed')
    }
    return new Promise((resolve, reject) => {
      axios
        .get(this.url, {
          headers: defaultHeaders,
          responseType: 'arraybuffer',
        })
        .then((response) => {
          this.type = response.headers['content-type'] as string
          this.data = Buffer.from(response.data, 'base64')
          this.s = sharp(this.data)
          this.s.metadata((err: any, metadata: any) => {
            if (err) reject(err)
            this.width = metadata.width
            this.height = metadata.height
          })
          resolve(this)
        })
        .catch((err) => {
          reject(err)
        })
    })
  }

  async processRequest(req: Request): Promise<Image> {
    return new Promise((resolve, reject) => {
      const q: any = req.query
      const h: any = req.headers

      this.resizeByQuery(q)
      this.performOperationsByQuery(q)
      this.convertFormatByAccept(q, h.accept)

      this.s?.toBuffer((err: any, buffer: Buffer) => {
        if (err) {
          reject(err)
        } else {
          this.data = buffer
          resolve(this)
        }
      })
    })
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
    )
    this.width = q.width ? Number(q.width) : undefined
    this.height = q.height ? Number(q.height) : undefined
  }

  convertFormatByAccept(q: any, accept: string): void {
    // return compatible format
    if (accept.includes('image/avif') && this.type !== 'image/gif') {
      this.type = 'image/avif'
      this.s?.avif({
        quality: (Number(q.quality) as number) || QUALITY,
        lossless: q.lossless !== undefined ? toBoolean(q.lossless) : undefined,
        effort: (Number(q.webpEffort) as number) || undefined,
      })
    } else if (accept.includes('image/webp')) {
      this.type = 'image/webp'
      this.s?.webp({
        quality: (Number(q.quality) as number) || QUALITY,
        lossless: q.lossless !== undefined ? toBoolean(q.lossless) : undefined,
        alphaQuality: (Number(q.alphaQuality) as number) || undefined,
        effort: (Number(q.webpEffort) as number) || undefined,
        loop: (Number(q.loop) as number) || undefined,
        delay: (Number(q.delay) as number) || undefined,
      })
    }
  }

  // eslint-disable-next-line class-methods-use-this
  performOperationsByQuery(q: any): void {
    if (q.blur && Number(q.blur) > 0 && Number(q.blur) <= 100) {
      this.s?.blur(Number(q.blur) as number)
    }
    if (q.sharpen && Number(q.sharpen) > 1 && Number(q.sharpen) <= 100) {
      this.s?.sharpen(Number(q.sharpen) as number)
    }
    if (q.flip) {
      this.s?.flip()
    }
    if (q.flop) {
      this.s?.flop()
    }
    if (q.rotate) {
      this.s?.rotate(Number(q.rotate) as number, {
        background: (q.background as string) || undefined,
      })
    }
  }

  setResponseHeaders(res: Response): void {
    res.setHeader('Content-Type', this.type as string)
    res.setHeader('Content-Length', this.data?.length || 0)
    res.setHeader('Cache-Control', 'public, max-age=31536000')
    res.setHeader('Expires', new Date(Date.now() + 31536000).toUTCString())
    res.setHeader('Last-Modified', new Date().toUTCString())
    res.setHeader('Pragma', 'public')
    res.setHeader('X-Powered-By', 'Image API')
  }
}

export default Image
export { Image }
