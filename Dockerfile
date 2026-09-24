# Build stage. It runs on the builder's own platform and cross-compiles
# for the target platform, so a multi-platform build (linux/amd64 and
# linux/arm64 in CI) needs no QEMU emulation. The runtime stage below
# must stay free of RUN steps for the same reason.
FROM --platform=$BUILDPLATFORM cgr.dev/chainguard/go AS build

ARG TARGETOS TARGETARCH
ARG VERSION

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
# Compute version from git: tagged release → `v1.17`, post-tag dev →
# `v1.17-3-gabc1234`. An explicit VERSION build argument takes precedence.
# The intentionally filtered Docker context omits tracked files, so asking
# git for a dirty suffix here would mark every image dirty. Falls back to
# "unknown" if .git is missing entirely (defensive: .git IS in the build
# context because .dockerignore deliberately retains it for this step).
#
# Injected into internal/buildinfo via `-ldflags -X` so both the OTel
# resource (service.version) and the PeeringDB User-Agent emit the
# same string. internal/buildinfo is the single source of truth.
RUN \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION=${VERSION:-$(git describe --tags --always 2>/dev/null || echo unknown)} && \
    echo "Building peeringdb-plus version=$VERSION for $TARGETOS/$TARGETARCH" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
        -trimpath \
        -ldflags="-s -w -X github.com/dotwaffle/peeringdb-plus/internal/buildinfo.injected=$VERSION" \
        -o /bin/peeringdb-plus ./cmd/peeringdb-plus

# Skeleton for the runtime /data dir: the static runtime image has no
# shell, so the directory is COPY'd in with ownership instead of RUN
# mkdir'd.
RUN mkdir /data-skeleton

# Runtime stage. chainguard/static has no libc, no shell and no package
# manager. The binary is CGO_ENABLED=0 and needs only what the image
# ships: the CA bundle, tzdata and the nonroot user (65532). Do not add a
# RUN step here: the build stage explains why. (Dockerfile.litefs keeps
# glibc-dynamic:latest-dev deliberately for incident response; see
# docs/DEPLOYMENT.md.)
FROM cgr.dev/chainguard/static

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
