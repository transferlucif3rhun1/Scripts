import { createClient } from 'redis';
import dotenv from 'dotenv';

dotenv.config();

const REDIS_HOST = process.env.REDIS_HOST || '127.0.0.1';
const REDIS_PORT = parseInt(process.env.REDIS_PORT || '6379', 10);

export const redisClient = createClient({
  socket: {
    host: REDIS_HOST,
    port: REDIS_PORT,
  },
});

redisClient.on('error', (err) => {
  console.error('Redis Client Error:', err);
});

export const connectRedis = async (): Promise<void> => {
  await redisClient.connect();
  console.log(`Redis connected on ${REDIS_HOST}:${REDIS_PORT}`);
};
