import express, { Application } from 'express'

import { PORT } from './utils/env'

import route from './routes/index'

const app: Application = express()
const cors = require('cors')

app.use(cors())
app.use(route)

app.listen(PORT, (): void => {
  // eslint-disable-next-line no-console
  console.log(`Server is running on port ${PORT}`)
})
