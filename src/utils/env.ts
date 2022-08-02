import dotenv, { DotenvConfigOptions } from 'dotenv';
import dotenvParseVariables, { Parsed } from 'dotenv-parse-variables';

let env: any = dotenv.config()
if (env.error) throw env.error;
env = dotenvParseVariables(env.parsed as Parsed);

const {
  PORT = 3000,
  LOSELESS = false,
  QUALITY = 80,
  FIT = 'outside',
} = env;

export {
  PORT,
  LOSELESS,
  QUALITY,
  FIT
};