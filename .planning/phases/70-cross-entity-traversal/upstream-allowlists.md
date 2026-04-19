# Upstream Allowlist Reference (serializers.py@99e92c72)

Source: `https://raw.githubusercontent.com/peeringdb/peeringdb/99e92c72/src/peeringdb_server/serializers.py`

Scratch working file for Phase 70 Plan 70-03. Cited by each
`pdbcompat.WithPrepareQueryAllow(...)` annotation via a `serializers.py:<LINE>`
comment.

The 13 upstream line anchors listed in CONTEXT.md coordination_notes are the
start lines of the 12 `prepare_query` classmethods plus the one
`finalize_query_params` classmethod on `NetworkSerializer` (line 2995):

| Upstream line | Serializer | Local ent type | PDB type |
|---------------|-----------|----------------|----------|
| 1823 | `FacilitySerializer.prepare_query` | `Facility` | `fac` |
| 2244 | `CarrierSerializer.prepare_query` | `Carrier` | `carrier` |
| 2361 | `InternetExchangeFacilitySerializer.prepare_query` | `IxFacility` | `ixfac` |
| 2423 | `NetworkContactSerializer.prepare_query` | `Poc` | `poc` |
| 2573 | `NetworkIXLanSerializer.prepare_query` | `NetworkIxLan` | `netixlan` |
| 2732 | `NetworkFacilitySerializer.prepare_query` | `NetworkFacility` | `netfac` |
| 2947 | `NetworkSerializer.prepare_query` | `Network` | `net` |
| 2995 | `NetworkSerializer.finalize_query_params` | `Network` (secondary) | `net` |
| 3315 | `IXLanPrefixSerializer.prepare_query` | `IxPrefix` | `ixpfx` |
| 3451 | `IXLanSerializer.prepare_query` | `IxLan` | `ixlan` |
| 3622 | `InternetExchangeSerializer.prepare_query` | `InternetExchange` | `ix` |
| 3925 | `CampusSerializer.prepare_query` | `Campus` | `campus` |
| 4041 | `OrganizationSerializer.prepare_query` | `Organization` | `org` |

## Semantics note

Upstream `get_relation_filters(flds, ...)` takes a list of RELATION SEED names
(not concrete `<fk>__<field>` keys). Clients submit
`?ix__name__contains=AMS-IX` and upstream's matcher accepts the key if `ix` is
in the seed list AND the suffix (`name`, `id`, `name_long`, etc.) is a real
column on the target model.

Our ent-side `WithPrepareQueryAllow` takes concrete `<fk>__<field>` keys (D-01
contract). Translation rule: for each upstream seed whose target is a PeeringDB
entity, enumerate the small set of client-useful filter columns on that
entity (`id`, `name`, + domain-specific fields like `asn`, `country`, `prefix`,
`fac_count`). The plan's `<context>` reference table (lines 106–119) encodes
this translation as design intent — we follow that list verbatim.

Upstream uses Django's reverse-accessor suffix `_set` (e.g.
`netfac_set__fac__name`). Our ent schemas declare forward edges
(`network.network_facilities`, `network_facility.facility`). Translate
`<reverse>_set__<field>` upstream tokens to the matching local forward edge
name. Example:

```
netfac_set__fac__name  →  network_facilities__facility__name
```

Cap enforced per D-04: every allowed key has ≤ 2 relation segments.

---

## FacilitySerializer (line 1823)

```python
# serializers.py:1820-1837
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("org")
    filters = get_relation_filters(
        [
            "net_id",
            "net",
            "ix_id",
            "ix",
            "org_name",
            "ix_count",
            "net_count",
            "carrier_count",
        ],
        cls,
        **kwargs,
    )
```

Local translation (→ `ent/schema/facility.go`):

- `org__name` (1-hop via `organization` edge — upstream `org_name` special-case at line 1844)
- `campus__name` (1-hop via `campus` edge — seed implied by `FacilitySerializer.Meta.related_fields = ["org", "campus"]` line 1816)
- `net__name` (1-hop via `network_facilities.network` — upstream seed `net`)
- `net__asn` (1-hop via `network_facilities.network.asn` — common client filter)
- `ix__name` (1-hop via `ix_facilities.ix_facility.internet_exchange` — upstream seed `ix`)
- `ix__id` (1-hop via `ix_facilities.ix_facility.internet_exchange.id`)
- `ixlan__ix__fac_count` (2-hop — the canonical test case from
  `pdb_api_test.py:5047,5081`; resolves via `ix_facilities.internet_exchange.fac_count`)

## CarrierSerializer (line 2244)

```python
# serializers.py:2243-2261
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.prefetch_related("org", "carrierfac_set")
    filters = get_relation_filters(
        [
            "carrierfac_set__facility_id",
        ],
        cls,
        **kwargs,
    )
```

Local translation (→ `ent/schema/carrier.go`):

- `org__name` (1-hop via `organization` edge — implied by
  `CarrierSerializer.Meta.related_fields = ["org", "carrierfac_set"]` line 2239)
- `fac__name` (1-hop via `carrier_facilities.facility` — upstream seed
  `carrierfac_set__facility_id` exposes the join; we expose the same reach
  via local edge `carrier_facilities.facility.name`)
- `fac__country` (1-hop via `carrier_facilities.facility.country`)

Note: upstream's literal `carrierfac_set__facility_id` seed gates passthrough
of `?carrierfac_set__facility_id=N` — we expose the same reach as
`fac__name` / `fac__country` since the PDB-compat surface rewrites `fac__*` to
the carrier_facilities.facility 2-hop path at parse time (Plan 70-05).

## InternetExchangeFacilitySerializer (line 2361)

```python
# serializers.py:2358-2369
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("ix", "ix__org", "facility")
    filters = get_relation_filters(["name", "country", "city"], cls, **kwargs)
    for field, e in list(filters.items()):
        for valid in ["name", "country", "city"]:
            if validate_relation_filter_field(field, valid):
                fn = getattr(cls.Meta.model, f"related_to_{valid}")
                field = f"facility__{valid}"
```

Local translation (→ `ent/schema/ixfacility.go`):

- `fac__name` (1-hop via `facility.name` — upstream seed `name` rewrites to
  `facility__name` at line 2366)
- `fac__country` (1-hop via `facility.country`)
- `fac__city` (1-hop via `facility.city`)
- `ix__name` (1-hop via `internet_exchange.name` — implied by
  `select_related("ix", ...)` and `related_fields = ["ix", "fac"]` line 2347)

## NetworkContactSerializer (line 2423)

```python
# serializers.py:2422-2425
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("network", "network__org")
    return qset, {}
```

Upstream does NOT call `get_relation_filters` here — it only eager-loads. The
client-facing filter surface is governed by what the parent
`ModelSerializer.prepare_query(...)` chain plus `queryable_relations` expose.
Per `related_fields = ["net"]` (line 2416), `net__*` filters are valid.

Local translation (→ `ent/schema/poc.go`):

- `net__name` (1-hop via `network.name`)
- `net__asn` (1-hop via `network.asn`)

## NetworkIXLanSerializer (line 2573)

```python
# serializers.py:2564-2585
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("network", "network__org")
    filters = get_relation_filters(["ix_id", "ix", "name"], cls, **kwargs)
    for field, e in list(filters.items()):
        for valid in ["ix", "name"]:
            if validate_relation_filter_field(field, valid):
                fn = getattr(cls.Meta.model, f"related_to_{valid}")
                if field == "name":
                    field = "ix__name"
```

Local translation (→ `ent/schema/networkixlan.go`):

- `net__name` (1-hop via `network.name` — implied by `select_related("network", ...)`)
- `net__asn` (1-hop via `network.asn`)
- `ix__name` (1-hop via `ix_lan.internet_exchange.name` — upstream seed `ix`
  routes through `ixlan__ix` via `related_to_ix` helper; we expose the
  1-hop-friendly alias the clients actually pass)
- `ix__id` (1-hop via `ix_lan.internet_exchange.id` — upstream seed `ix_id`)
- `ixlan__name` (1-hop via `ix_lan.name`)

## NetworkFacilitySerializer (line 2732)

```python
# serializers.py:2729-2741
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("network", "network__org")
    filters = get_relation_filters(["name", "country", "city"], cls, **kwargs)
    for field, e in list(filters.items()):
        for valid in ["name", "country", "city"]:
            if validate_relation_filter_field(field, valid):
                fn = getattr(cls.Meta.model, f"related_to_{valid}")
                field = f"facility__{valid}"
```

Local translation (→ `ent/schema/networkfacility.go`):

- `net__name` (1-hop via `network.name` — implied by `select_related("network", ...)`)
- `net__asn` (1-hop via `network.asn`)
- `fac__name` (1-hop via `facility.name` — upstream seed `name` rewrites to
  `facility__name` at line 2737)
- `fac__country` (1-hop via `facility.country`)

## NetworkSerializer (line 2947)

```python
# serializers.py:2938-2964
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("org")
    filters = get_relation_filters(
        [
            "ixlan_id",
            "ixlan",
            "ix_id",
            "ix",
            "netixlan_id",
            "netixlan",
            "netfac_id",
            "netfac",
            "fac",
            "fac_id",
            "fac_count",
            "ix_count",
        ],
        cls,
        **kwargs,
    )
```

(Secondary line 2995: `NetworkSerializer.finalize_query_params` — translates
legacy `info_type=X` into `info_types=X` annotation split; no allowlist keys
emit from there. Comment references both lines for audit completeness.)

Local translation (→ `ent/schema/network.go`):

- `org__name` (1-hop via `organization.name` — implied by `select_related("org")`)
- `org__id` (1-hop via `organization.id`)
- `ix__name` (1-hop via `network_ix_lans.ix_lan.internet_exchange.name` — upstream seed `ix`)
- `ixlan__name` (1-hop via `network_ix_lans.ix_lan.name` — upstream seed `ixlan`)
- `fac__name` (1-hop via `network_facilities.facility.name` — upstream seed `fac`)
- `network_facilities__facility__name` (2-hop via forward-edge path —
  translation of upstream `netfac_set__fac__name` reverse-accessor)

## NetworkSerializer.finalize_query_params (line 2995)

```python
# serializers.py:2994-3001+
@classmethod
def finalize_query_params(cls, qset, query_params: dict):
    # legacy info_type field needs to be converted to info_types
    # we do this by creating an annotation based on the info_types split by ','
    # ...
```

Secondary NetworkSerializer hook — translates the legacy scalar `info_type=X`
query param into an `info_types`-list contains match via Django Q objects. No
new relation-filter keys emitted; Network's allowlist already captured in its
primary `prepare_query` at line 2947. Cited on the Network annotation for
audit-trace completeness (so the reviewer sees both upstream sites when
diffing).

## IXLanPrefixSerializer (line 3315)

```python
# serializers.py:3315-3329
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("ixlan", "ixlan__ix", "ixlan__ix__org")
    filters = get_relation_filters(["ix_id", "ix", "whereis"], cls, **kwargs)
    for field, e in list(filters.items()):
        for valid in ["ix"]:
            if validate_relation_filter_field(field, valid):
                fn = getattr(cls.Meta.model, f"related_to_{valid}")
```

Local translation (→ `ent/schema/ixprefix.go`):

- `ixlan__name` (1-hop via `ix_lan.name` — implied by
  `select_related("ixlan", ...)`)
- `ixlan__ix__name` (2-hop via `ix_lan.internet_exchange.name` — upstream seed
  `ix` routes through `ixlan__ix` per `select_related("ixlan", "ixlan__ix", ...)`)
- `ixlan__ix__id` (2-hop via `ix_lan.internet_exchange.id` — upstream seed
  `ix_id`)

DROP: `whereis` — not a relation traversal, it's a special
IP-address-in-prefix spatial search (line 3327,
`Model.whereis_ip(value, qset=qset)`). Handled separately in pdbcompat if
needed (out of Phase 70 scope).

## IXLanSerializer (line 3451)

```python
# serializers.py:3450-3452
@classmethod
def prepare_query(cls, qset, **kwargs):
    return qset.select_related("ix", "ix__org"), {}
```

Upstream does NOT call `get_relation_filters` — only eager-loads `ix` and
`ix__org`. Per `related_fields = ["ix"]` (line 3444), `ix__*` filters are
valid. Parent `InternetExchange` is the only FK.

Local translation (→ `ent/schema/ixlan.go`):

- `ix__name` (1-hop via `internet_exchange.name`)
- `ix__id` (1-hop via `internet_exchange.id`)
- `ixpfx__prefix` (1-hop via `ix_prefixes.prefix` — reverse FK via
  `IxPrefix.ixlan_id`; exposed because clients commonly filter IxLans by the
  prefix they contain)

## InternetExchangeSerializer (line 3622)

```python
# serializers.py:3621-3641
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("org")
    filters = get_relation_filters(
        [
            "ixlan_id",
            "ixlan",
            "ixfac_id",
            "ixfac",
            "fac_id",
            "fac",
            "net_id",
            "net",
            "net_count",
            "fac_count",
            "capacity",
        ],
        cls,
        **kwargs,
    )
```

Local translation (→ `ent/schema/internetexchange.go`):

- `org__name` (1-hop via `organization.name` — implied by `select_related("org")`)
- `ixlan__name` (1-hop via `ix_lans.name` — upstream seed `ixlan`)
- `ixpfx__prefix` (1-hop via `ix_lans.ix_prefixes.prefix` — client-facing
  filter; reach through ixlan)
- `net__name` (1-hop via `ix_lans.network_ix_lans.network.name` — upstream seed `net`)
- `net__asn` (1-hop via `ix_lans.network_ix_lans.network.asn`)
- `fac__name` (1-hop via `ix_facilities.facility.name` — upstream seed `fac`)
- `fac__country` (1-hop via `ix_facilities.facility.country`)

DROP: `capacity` — special aggregator (`Model.filter_capacity` line 3665), not
a relation field. `ixfac__*` and `ixfac_id` — pass-through to
`ix_facilities.id` already covered by direct `ix_facilities.id` filter in
entrest; no new 2-hop path added.

## CampusSerializer (line 3925)

```python
# serializers.py:3924-3940
@classmethod
def prepare_query(cls, qset, **kwargs):
    qset = qset.select_related("org")
    filters = get_relation_filters(["facility"], cls, **kwargs)
    for field, e in list(filters.items()):
        field = field.replace("facility", "fac_set")
        fn = getattr(cls.Meta.model, "related_to_facility")
```

Local translation (→ `ent/schema/campus.go`):

- `org__name` (1-hop via `organization.name` — implied by `select_related("org")`)
- `fac__name` (1-hop via `facilities.name` — upstream seed `facility` rewrites
  to `fac_set__...` at line 3936; our forward edge is `facilities`, exposed as
  PDB-surface alias `fac`)
- `fac__country` (1-hop via `facilities.country`)

## OrganizationSerializer (line 4041)

```python
# serializers.py:4040-4063
@classmethod
def prepare_query(cls, qset, **kwargs):
    filters = {}
    if "asn" in kwargs:
        asn = kwargs.get("asn", [""])[0]
        qset = qset.filter(net_set__asn=asn, net_set__status="ok")
        filters.update({"asn": kwargs.get("asn")})
    # ... spatial search / distance
    return qset, filters
```

Upstream does NOT call `get_relation_filters` — it only handles the special
`asn` → `net_set__asn` rewrite. The relation filter surface is governed by
`queryable_relations` auto-introspection (Path B).

Local translation (→ `ent/schema/organization.go`):

- `net__name` (1-hop via `networks.name`)
- `net__asn` (1-hop via `networks.asn` — upstream `asn` special-case)
- `ix__name` (1-hop via `internet_exchanges.name`)
- `fac__name` (1-hop via `facilities.name`)
- `fac__country` (1-hop via `facilities.country`)

DROP: `distance` — spatial search, not a relation filter (handled via
`convert_to_spatial_search` line 4056; out of Phase 70 scope).
