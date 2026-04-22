# GEMINI.md

This file provides instructional context for the `peeringdb-plus` project to facilitate AI-assisted development.

## Project Overview
`peeringdb-plus` is a high-performance, globally distributed, read-only mirror of [PeeringDB](https://www.peeringdb.com) data. It synchronizes PeeringDB objects on a configurable schedule, stores them in SQLite (using LiteFS for edge replication on Fly.io), and serves the data through five API surfaces.

## API Surfaces
- **Web UI:** `/ui/` (Search, comparison, and detail pages)
- **GraphQL:** `/graphql` (Playground and query endpoint)
- **REST:** `/rest/v1/` (OpenAPI-compliant)
- **PeeringDB Compat API:** `/api/` (Drop-in replacement for PeeringDB API)
- **ConnectRPC:** `/peeringdb.v1.*/` (Get, List, and Stream RPCs for 13 entity types)

## Development Environment
- **Language:** Go 1.26+
- **Database:** SQLite (via pure-Go `modernc.org/sqlite`)
- **Infrastructure:** Fly.io (with LiteFS for replication)
- **Core Technologies:**
  - ORM: `entgo`
  - RPC: ConnectRPC (gRPC/Connect/gRPC-Web)
  - Monitoring: OpenTelemetry (traces, metrics, logs)
  - Web: `templ` + `htmx` + Tailwind CSS
  - GraphQL: `gqlgen` (via entgql)

## Key Commands
- **Build All:** `go build ./...`
- **Run Locally:** `go build -o peeringdb-plus ./cmd/peeringdb-plus && ./peeringdb-plus`
- **Generate:** `go generate ./...` (Runs the full codegen pipeline for `ent`, `templ`, and `proto`)
- **Test:** `go test -race ./...`
- **Lint/Check:**
  - `go vet ./...`
  - `golangci-lint run`
  - `govulncheck ./...`

## Development Conventions
- **Configuration:** All configuration is via environment variables (prefixed `PDBPLUS_`).
- **Syncing:** Syncs automatically (default: hourly, `PDBPLUS_SYNC_INTERVAL`).
- **Codegen:** Uses `ent`, `templ`, `gqlgen`, and `protobuf`. `go generate` is essential after schema or proto changes.
- **Minimal Dependencies:** Built for container efficiency; prefers pure Go implementations.

## Infrastructure & Deployment
- **Base Images:** Chainguard (minimal).
- **Deployment:** Managed via `fly.toml` for Fly.io.
- **Replication:** LiteFS is used for distributing the SQLite database across edges.
