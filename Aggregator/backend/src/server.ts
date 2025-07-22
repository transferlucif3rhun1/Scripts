import dotenv from 'dotenv';
dotenv.config();

import express from 'express';
import cors from 'cors';
import { authMiddleware } from './middleware/authMiddleware';
import { connectDB } from './config/db';
import { connectRedis, redisClient } from './config/redisClient';

import http from 'http';
import { Server as SocketIOServer } from 'socket.io';

import { ApolloServer } from '@apollo/server';
import { expressMiddleware } from '@apollo/server/express4';

import { typeDefs, resolvers } from './graphql';
import { logger } from './utils/logger';

async function startServer() {
  await connectDB();
  await connectRedis();

  const app = express();
  app.use(cors());
  app.use(express.json());

  // Attach auth middleware
  app.use(authMiddleware);

  // Create HTTP server + Socket.io
  const httpServer = http.createServer(app);
  const io = new SocketIOServer(httpServer, {
    cors: {
      origin: '*',
    },
  });

  io.on('connection', (socket) => {
    logger.info(`New client connected: ${socket.id}`);
    socket.on('disconnect', () => {
      logger.info(`Client disconnected: ${socket.id}`);
    });
  });

  // Apollo Server
  const apolloServer = new ApolloServer({
    typeDefs,
    resolvers,
  });
  await apolloServer.start();

  app.use(
    '/graphql',
    expressMiddleware(apolloServer, {
      context: async ({ req }) => {
        // Provide "io" to resolvers for real-time notifications
        return {
          currentUser: req.currentUser,
          redisClient,
          io,
        };
      },
    })
  );

  const PORT = process.env.PORT || 4000;
  httpServer.listen(PORT, () => {
    logger.info(`Server is running on http://localhost:${PORT}/graphql`);
  });
}

startServer().catch((err) => {
  console.error('Error starting server:', err);
});
