# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /peeringdb-plus ./cmd/peeringdb-plus

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates

LABEL org.opencontainers.image.title="peeringdb-plus" \
      org.opencontainers.image.description="High-performance read-only PeeringDB mirror" \
      org.opencontainers.image.source="https://github.com/dotwaffle/peeringdb-plus"

COPY --from=builder /peeringdb-plus /usr/local/bin/peeringdb-plus

RUN mkdir -p /data
ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q --spider http://localhost:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]
