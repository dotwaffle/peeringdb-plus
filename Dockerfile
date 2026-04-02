# Build stage
FROM cgr.dev/chainguard/go AS build

WORKDIR /app
COPY . .
RUN \
    --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/peeringdb-plus ./cmd/peeringdb-plus

# Runtime stage
FROM cgr.dev/chainguard/glibc-dynamic:latest-dev

COPY --from=build /bin/peeringdb-plus /usr/local/bin/peeringdb-plus

USER root
RUN mkdir -p /data && chown nonroot:nonroot /data
USER nonroot
ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db

LABEL org.opencontainers.image.title="peeringdb-plus" \
      org.opencontainers.image.description="High-performance read-only PeeringDB mirror" \
      org.opencontainers.image.source="https://github.com/dotwaffle/peeringdb-plus"

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]
