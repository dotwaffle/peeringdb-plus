---
name: peeringdb-plus
description: Query the PeeringDB Plus read-only mirror for networks, exchanges, facilities, organizations, campuses, carriers, IX peering addresses, network comparisons, and sync freshness. Use for PeeringDB research, interconnection discovery, network footprint analysis, or mirror health checks.
---

# PeeringDB Plus

Use the PeeringDB Plus MCP server for read-only PeeringDB research.

## Start

1. Read `peeringdb-plus://service` for the server version and serving region.
2. Read `peeringdb-plus://guide` before you page through related records.
3. Use `get_sync_status` when freshness affects the answer.

## Choose a tool

- Use `search_peeringdb` to find records across object types.
- Use `get_network`, `get_exchange`, `get_facility`, `get_organization`,
  `get_campus`, or `get_carrier` for a known record.
- Use `compare_networks` to compare network footprints.
- Use `lookup_ip` to find the network that holds an IX peering address and
  the exchange prefix that contains it. It does not map other IP addresses to
  networks.
- Use the `research_network` prompt for a structured network investigation.
- Use the `compare_networks` prompt for a guided comparison.

## Work with results

- Set `relation` to fetch one related collection, and set `page_size` to limit
  the rows.
- Follow relation cursors when a response indicates more related records.
- Treat the data as a mirror snapshot and report its freshness when material.
- Distinguish returned facts from inferences, and preserve source record IDs in
  the answer.
