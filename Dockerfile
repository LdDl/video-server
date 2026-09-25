# syntax=docker/dockerfile:1
FROM docker.io/golang:1.25.9-alpine3.23 AS builder

WORKDIR /build_app

ENV GOTOOLCHAIN=local
ENV GOPROXY=https://proxy.golang.org

# Download dependencies before copying application sources.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY ./*.go ./
COPY ./configuration ./configuration
COPY ./storage ./storage
COPY ./cmd ./cmd

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -mod=readonly -buildvcs=false -trimpath -ldflags="-s -w" -o video_server ./cmd/video_server

FROM scratch

WORKDIR /app

# Needed only when the archive storage talks to S3/MinIO over TLS.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build_app/video_server ./

# 8090 serves MSE websockets and HLS files, 8091 serves the REST API.
EXPOSE 8090 8091

ENTRYPOINT ["./video_server"]
