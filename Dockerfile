FROM golang:1.23 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o mcp-proxy main.go

FROM ghcr.io/astral-sh/uv:debian-slim 
ENV DEBIAN_FRONTEND=noninteractive \
    NODE_VERSION=lts.x
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl ca-certificates && \
    curl -fsSL --fail https://deb.nodesource.com/setup_${NODE_VERSION} | bash - && \
    apt-get update && \
    apt-get install -y --no-install-recommends nodejs && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/mcp-proxy /main
ENTRYPOINT ["/main"]
CMD ["--config", "/config/config.json"]
