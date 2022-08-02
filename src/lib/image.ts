import axios from 'axios'
import { Request, Response } from 'express'
import sharp, { FitEnum, Sharp } from 'sharp'
import { QUALITY, FIT } from '../utils/env'
import { toBoolean } from '../utils/helpers'

const defaultHeaders = {
  'User-Agent':
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36',
  Accept: '*/*',
  'Accept-Encoding': 'gzip, deflate, br',
  Connection: 'keep-alive',
  'Upgrade-Insecure-Requests': '1',
}

type ImageObject = {
  url: string
  type: string
  data: string
}

class Image {
  url: string

  type: string | undefined

  data: Buffer | undefined

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
    return new Promise((resolve, reject) => {
      axios
        .get(this.url, {
          headers: defaultHeaders,
          responseType: 'arraybuffer',
        })
        .then((response) => {
          this.type = response.headers['content-type'] as string
          this.data = Buffer.from(response.data, 'base64')
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
      const s = sharp(this.data)

      this.resizeByQuery(s, q)
      this.performOperationsByQuery(s, q)
      this.convertFormatByAccept(s, q, h.accept)

      s.toBuffer((err: any, buffer: Buffer) => {
        if (err) {
          reject(err)
        } else {
          this.data = buffer
          resolve(this)
        }
      })
    })
  }

  resizeByQuery(s: Sharp, q: any): void {
    s.resize(
      parseInt(q.width as string) || undefined,
      parseInt(q.height as string) || undefined,
      {
        fit: q.fit || (FIT as string),
        position: (q.position as string) || undefined,
        kernel: q.kernel || undefined,
        background: (q.background as string) || undefined,
        // withoutEnlargement: q.withoutEnlargement as boolean || undefined,
        // withoutReduction: q.withoutReduction as boolean || undefined,
        // fastShrinkOnLoad: q.fastShrinkOnLoad as boolean || undefined,
      }
    )
  }

  convertFormatByAccept(s: Sharp, q: any, accept: string): void {
    // return compatible format
    if (accept.includes('image/avif') && this.type != 'image/gif') {
      this.type = 'image/avif'
      s.avif({
        quality: (parseInt(q.quality) as number) || QUALITY,
        lossless: q.lossless != undefined ? toBoolean(q.lossless) : undefined,
        effort: (parseInt(q.webpEffort) as number) || undefined,
      })
    } else if (accept.includes('image/webp')) {
      this.type = 'image/webp'
      s.webp({
        quality: (parseInt(q.quality) as number) || QUALITY,
        lossless: q.lossless != undefined ? toBoolean(q.lossless) : undefined,
        alphaQuality: (parseInt(q.alphaQuality) as number) || undefined,
        effort: (parseInt(q.webpEffort) as number) || undefined,
        loop: (parseInt(q.loop) as number) || undefined,
        delay: (parseInt(q.delay) as number) || undefined,
      })
    }
  }

  performOperationsByQuery(s: Sharp, q: any): void {
    if (q.blur && parseInt(q.blur) > 0 && parseInt(q.blur) <= 100) {
      s.blur(parseInt(q.blur) as number)
    }
    if (q.sharpen && parseInt(q.sharpen) > 1 && parseInt(q.sharpen) <= 100) {
      s.sharpen(parseInt(q.sharpen) as number)
    }
    if (q.flip) {
      s.flip()
    }
    if (q.flop) {
      s.flop()
    }
    if (q.rotate) {
      s.rotate(parseInt(q.rotate) as number, {
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
export { Image, ImageObject }
