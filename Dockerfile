FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o openswiss ./cmd/openswiss

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/openswiss .
COPY --from=builder /build/migrations ./migrations
COPY --from=builder /build/templates ./templates
COPY --from=builder /build/static ./static

EXPOSE 8080

ENTRYPOINT ["./openswiss"]
