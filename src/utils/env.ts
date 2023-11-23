import dotenv, { DotenvConfigOptions } from 'dotenv';
import dotenvParseVariables, { Parsed } from 'dotenv-parse-variables';
import fs from 'fs';

let env: any;
// check if .env file exists
if (fs.existsSync('.env')) {
  env = dotenv.config({ silent: true } as DotenvConfigOptions);
  if (env.error) throw env.error;
  env = dotenvParseVariables(env.parsed as Parsed);
} else {
  // load envs from process.env
  env = dotenvParseVariables(process.env as any);
}

const {
  ENVIRONMENT = 'production',
  API_ENDPOINT = '/api/image',
  PORT = 3000,
  LOSELESS = false,
  QUALITY = 80,
  FIT = 'outside',
  LOG_LEVEL = 'info',
  ORIGIN_WHITELIST = '*',
  REDIRECT_ON_ERROR = false,
} = env;

export {
  ENVIRONMENT,
  API_ENDPOINT,
  PORT,
  LOSELESS,
  QUALITY,
  FIT,
  LOG_LEVEL,
  ORIGIN_WHITELIST,
  REDIRECT_ON_ERROR,
};
