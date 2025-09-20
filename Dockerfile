# Multi-stage build for listmonk
# Stage 1: Build stage
FROM node:18-alpine AS frontend-builder

# Prevent Cypress from downloading its heavy binary during install (saves space/time in prod builds)
ENV CYPRESS_INSTALL_BINARY=0

# Install yarn
RUN apk add --no-cache yarn

# Set working directory
WORKDIR /app

# Copy frontend files
COPY frontend/package.json frontend/yarn.lock ./frontend/
COPY frontend/email-builder/package.json frontend/email-builder/yarn.lock ./frontend/email-builder/

# Copy static directory for postinstall script
COPY static/ ./static/

# Copy gitignore for eslint
COPY .gitignore ./

# Install frontend dependencies
RUN cd frontend && yarn install --frozen-lockfile
RUN cd frontend/email-builder && yarn install --frozen-lockfile

# Copy frontend source code
COPY frontend/ ./frontend/
COPY frontend/email-builder/ ./frontend/email-builder/

# Copy gitignore for eslint
RUN cp .gitignore ./frontend/.gitignore

# Build frontend
RUN cd frontend && yarn build
RUN cd frontend/email-builder && yarn build
RUN mkdir -p frontend/public/static/email-builder && cp -r frontend/email-builder/dist/* frontend/public/static/email-builder/

# Stage 2: Go builder
FROM golang:1.24-alpine AS go-builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Copy built frontend from previous stage
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY --from=frontend-builder /app/frontend/public/static/email-builder ./frontend/public/static/email-builder

# Build the binary
RUN CGO_ENABLED=0 go build -o listmonk -ldflags="-s -w" cmd/*.go

# Install stuffbin
RUN go install github.com/knadh/stuffbin/...

# Pack static files into binary using stuffbin
RUN stuffbin -a stuff -in listmonk -out listmonk \
    config.toml.sample \
    schema.sql queries.sql permissions.json \
    static/public:/public \
    static/email-templates \
    frontend/dist:/admin \
    frontend/public/static/email-builder:/admin/static/email-builder \
    i18n:/i18n

# Stage 3: Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata shadow su-exec

# Set the working directory
WORKDIR /listmonk

# Copy the binary from builder stage
COPY --from=go-builder /app/listmonk .

# Copy the entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/

# Make the entrypoint script executable
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Create uploads directory
RUN mkdir -p uploads

# Expose the application port
EXPOSE 9000

# Set the entrypoint
ENTRYPOINT ["docker-entrypoint.sh"]

# Define the command to run the application
CMD ["./listmonk"]
