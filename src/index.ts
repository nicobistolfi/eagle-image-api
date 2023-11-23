import express, { Application } from 'express';

import { PORT } from './utils/env';

import route from './routes/index';
import { info } from './utils/logger';

const app: Application = express();
const cors = require('cors');

app.use(cors());
app.use(route);

app.listen(PORT, (): void => {
  info(`Server is running on port ${PORT}`);
});
