# Stage 1: Build
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY tsconfig.json vite.config.ts ./
COPY public ./public
COPY src ./src
RUN npm run build

# Stage 2: Serve with Node or a simple static server
FROM node:18-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
RUN npm install -g serve
EXPOSE 3000
CMD ["serve", "-s", "dist"]
