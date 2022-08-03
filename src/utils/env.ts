import dotenv, { DotenvConfigOptions } from 'dotenv'
import dotenvParseVariables, { Parsed } from 'dotenv-parse-variables'
import fs from 'fs'

let env: any
// check if .env file exists
if (fs.existsSync('.env')) {
  env = dotenv.config({ silent: true } as DotenvConfigOptions)
  if (env.error) throw env.error
  env = dotenvParseVariables(env.parsed as Parsed)
} else {
  // load dotenv from process.env
  env = dotenvParseVariables(process.env as any)
}

const {
  PORT = 3000, LOSELESS = false, QUALITY = 80, FIT = 'outside',
} = env

export {
  PORT, LOSELESS, QUALITY, FIT,
}
