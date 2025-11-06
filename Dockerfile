# Build stage
FROM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY *.go ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-w -s" -trimpath -o tsddns .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/tsddns /app/tsddns

RUN mkdir -p /config && \
    addgroup -g 1000 tsddns && \
    adduser -D -u 1000 -G tsddns tsddns && \
    chown -R tsddns:tsddns /app /config

USER tsddns

ENTRYPOINT ["/app/tsddns"]
CMD ["--config", "/config/config.json"]
