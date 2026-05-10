FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

# Templates, static assets, and migrations are baked into the binary via
# go:embed, so we just need the full source tree to build.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o openswiss .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates && \
    addgroup -S openswiss && \
    adduser -S -G openswiss -h /app -s /sbin/nologin openswiss

WORKDIR /app

COPY --from=builder --chown=openswiss:openswiss /build/openswiss .

USER openswiss

EXPOSE 8080

ENTRYPOINT ["./openswiss"]
