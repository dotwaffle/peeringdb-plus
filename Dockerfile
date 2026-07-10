# Build stage
FROM cgr.dev/chainguard/go AS build

WORKDIR /app

# Module download in its own layer, keyed on go.mod/go.sum only, and
# deliberately WITHOUT a cache mount: BuildKit cache-mount contents are
# builder-local and are NOT exported by the CI `type=gha` cache, so
# modules baked into a real layer are what make CI rebuilds skip the
# download. The go-build cache mount below still helps local iterative
# builds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/peeringdb-plus ./cmd/peeringdb-plus

# Skeleton for the runtime /data dir: the plain (non-dev) runtime image
# has no shell, so the directory is COPY'd in with ownership instead of
# RUN mkdir'd.
RUN mkdir /data-skeleton

# Runtime stage. Plain glibc-dynamic (not :latest-dev): the dev variant
# adds a shell + apk that this image never needs — it installs nothing
# and runs nonroot. (Dockerfile.prod keeps -dev deliberately for
# incident response; see docs/DEPLOYMENT.md.)
FROM cgr.dev/chainguard/glibc-dynamic

COPY --from=build /bin/peeringdb-plus /usr/local/bin/peeringdb-plus
# 65532 is the chainguard nonroot uid/gid.
COPY --from=build --chown=65532:65532 /data-skeleton /data

USER nonroot
ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db

LABEL org.opencontainers.image.title="peeringdb-plus" \
      org.opencontainers.image.description="High-performance read-only PeeringDB mirror" \
      org.opencontainers.image.source="https://github.com/dotwaffle/peeringdb-plus"

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]
