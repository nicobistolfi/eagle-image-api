import express, { Application } from 'express'

import dotenv, { DotenvConfigOptions } from 'dotenv'
import dotenvParseVariables, { Parsed } from 'dotenv-parse-variables'

import route from './routes/index'

const app: Application = express()
const cors = require('cors')

let env: any = dotenv.config({ silent: true } as DotenvConfigOptions)
// if (env.error) throw env.error
env = dotenvParseVariables(env.parsed as Parsed)

const PORT: number = env.PORT || 3000

app.use(cors())
app.use(route)

app.listen(PORT, (): void => {
  // eslint-disable-next-line no-console
  console.log(`Server is running on port ${PORT}`)
})
