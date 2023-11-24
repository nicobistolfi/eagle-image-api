/* eslint-disable no-underscore-dangle */
import winston, { Logger } from 'winston';
import util from 'util';
import { LOG_LEVEL } from './env';

const logger: Logger = winston.createLogger({
  level: LOG_LEVEL,
  format: winston.format.simple(),
  transports: [new winston.transports.Console()],
});

logger.format = winston.format.combine(
  winston.format.colorize(),
  winston.format.timestamp(),
  winston.format.printf((i: any) => {
    const { timestamp, level, message } = i;
    return `${timestamp} [${level}]: ${message}`;
  }),
);

function debug(message: string, ...args: any[]) {
  logger.log('debug', util.format(message, ...args));
}

function info(message: string, ...args: any[]) {
  logger.log('info', util.format(message, ...args));
}

function error(message: string, ...args: any[]) {
  logger.log('error', util.format(message, ...args));
}

function silly(message: string, ...args: any[]) {
  logger.log('silly', util.format(message, ...args));
}

export default logger;
export { info, debug, error, silly };
