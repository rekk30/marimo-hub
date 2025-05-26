  FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
  WORKDIR /src
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  RUN GOOS=linux GOARCH=amd64 go build -o /out/marimo-hub ./cmd/main.go

  FROM python:3.13-slim

  RUN apt-get update && apt-get install -y \
      curl \
      git \
      build-essential \
      && rm -rf /var/lib/apt/lists/*

  RUN pip install --upgrade pip && pip install marimo

  RUN mkdir -p /notebooks /data

  WORKDIR /notebooks

  COPY --from=builder /out/marimo-hub /app/marimo-hub

  EXPOSE 80 8080 8081
  ENV API_PORT=8081
  ENV MARIMO_PORT=8080
  ENV PROXY_PORT=80
  ENV NOTEBOOKS_PATH=/notebooks
  ENV DB_PATH=/data/marimo-hub.db
  ENV NOTEBOOK_PORT_RANGE=3000-4000

  CMD ["/app/marimo-hub"]
