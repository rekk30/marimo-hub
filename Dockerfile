# Use Python as base image since we need marimo
FROM python:3.13-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install uv and add it to PATH
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    echo 'export PATH="/root/.local/bin:$PATH"' >> /root/.bashrc && \
    export PATH="/root/.local/bin:$PATH" && \
    uv pip install --system marimo

# Install Go
RUN curl -OL https://go.dev/dl/go1.22.1.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz \
    && rm go1.22.1.linux-amd64.tar.gz

# Add Go to PATH and set environment variables for cross-compilation
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOARCH=arm64
ENV GOOS=linux
ENV CGO_ENABLED=0

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download Go dependencies
RUN go mod download

# Copy the rest of the application
COPY . .

# Build the application
RUN go build -o marimo-hub ./cmd/main.go

# Create directories for notebooks and database
RUN mkdir -p /notebooks /data

# Expose ports
# 80: proxy server
# 8080: marimo
# 8081: API server
EXPOSE 80 8080 8081

# Set environment variables with defaults
ENV API_PORT=8081
ENV MARIMO_PORT=8080
ENV PROXY_PORT=80
ENV NOTEBOOKS_PATH=/notebooks
ENV DB_PATH=/data/marimo-hub.db
ENV NOTEBOOK_PORT_RANGE=3000-4000

# Run the application
CMD ["./marimo-hub"]