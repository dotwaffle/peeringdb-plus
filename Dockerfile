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
# `go build` stamps the main module version from git (Go 1.24 and
# later): the tag on a tagged commit, a pseudo-version between tags, and
# a +dirty suffix when the context differs from the commit. .dockerignore
# keeps .git and every tracked file in the context for this.
# internal/buildinfo reads the stamp, so the OTel resource
# (service.version) and the PeeringDB User-Agent emit the same string.
# An explicit VERSION build argument overrides the stamp, for a context
# without .git. `go version -m` prints the stamp into the build log.
RUN \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
        -trimpath \
        -ldflags="-s -w ${VERSION:+-X github.com/dotwaffle/peeringdb-plus/internal/buildinfo.injected=$VERSION}" \
        -o /bin/peeringdb-plus ./cmd/peeringdb-plus && \
    go version -m /bin/peeringdb-plus | grep -E '^[[:space:]]+(mod|build[[:space:]]+vcs)'

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
