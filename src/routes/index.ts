import axios from 'axios'
import express, { Response, Request, Router } from 'express'
import { Image, ImageObject } from '../lib/image'

const route = Router()
const path = require('path')

route.get('/api/v1/image', async (req: Request, res: Response) => {
  const image = new Image(req.query.url as string)
  console.log(await image.isImage())
  await image.loadImage()
  await image.processRequest(req)
  // res.send(imageObject.data);
  image.setResponseHeaders(res)
  // let headers: object = image.setResponseHeaders();
  // res.writeHead(200, headers);
  res.end(image.data)
  // res.send(Buffer.from(imageObject.data, 'base64'));
  // image.getImage(req.query.url).then((data: any) => {
  //   res.send(data);
  // }).catch((err: any) => {
  //   res.send(err);
  // });

  // res.send('Hello World');
})

export default route
