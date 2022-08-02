import express, { Application } from 'express';
const app: Application = express();
const cors = require('cors');

import dotenv from 'dotenv';
import dotenvParseVariables, { Parsed } from 'dotenv-parse-variables';

let env: any = dotenv.config({})
if (env.error) throw env.error;
env = dotenvParseVariables(env.parsed as Parsed);

import route from './routes/index';

const PORT: number = env.PORT || 3000;

app.use(cors());
app.use(route);

app.listen(PORT, (): void => {
  console.log(`Server is running on port ${PORT}`);
});