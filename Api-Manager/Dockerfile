FROM golang:1.21-alpine AS backend-builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY main.go ./
COPY server.json ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Frontend build stage
FROM node:18-alpine AS frontend-builder

WORKDIR /app

# Copy package files
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy the rest of the application
COPY public/ public/
COPY src/ src/
COPY tsconfig.json postcss.config.js tailwind.config.js ./

# Build the frontend application
RUN npm run build

# Final stage
FROM alpine:3.18

WORKDIR /app

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy the built binary from the backend builder
COPY --from=backend-builder /app/main .
COPY --from=backend-builder /app/server.json .

# Copy the built frontend from the frontend builder
COPY --from=frontend-builder /app/build ./public

# Expose the port
EXPOSE 8080

# Run the application
CMD ["./main"]