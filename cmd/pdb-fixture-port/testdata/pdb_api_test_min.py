# Minimal stub mirroring upstream pdb_api_test.py fixture DSL.
# Shape is identical to the real file; content is trimmed for
# deterministic testing of cmd/pdb-fixture-port parser.
#
# Real upstream lives at:
#   peeringdb/peeringdb@<SHA>:src/peeringdb_server/management/commands/pdb_api_test.py

ORG_RW = "API Test Organization RW"
ORG_R = "API Test Organization R"


def test_user_001_GET_list_filter_country_exact(self):
    ix_li = InternetExchange.objects.create(
        status="ok", name="Test IX Liechtenstein", country="LI"
    )
    ix_be = InternetExchange.objects.create(
        status="ok", name="Test IX Belgium", country="BE"
    )
    ix_bo = InternetExchange.objects.create(
        status="ok", name="Test IX Bolivia", country="BO"
    )


def test_ordering_001(self):
    # Rows with varying updated/created — feeds (-updated, -created) order check.
    n1 = Network.objects.create(
        status="ok", name="OrderNet-A", asn=65001,
        updated="2024-01-01T00:00:00Z", created="2024-01-01T00:00:00Z"
    )
    n2 = Network.objects.create(
        status="ok", name="OrderNet-B", asn=65002,
        updated="2024-02-01T00:00:00Z", created="2024-01-01T00:00:00Z"
    )
    o1 = Organization.objects.create(
        status="ok", name="OrderOrg-A",
        updated="2024-03-01T00:00:00Z", created="2024-01-01T00:00:00Z"
    )


def test_status_matrix_001(self):
    # STATUS-01..05 fixture surface — explicit status assignments
    # spanning ok / pending / deleted across multiple entity types.
    # Campus pending row exercises STATUS-03 carve-out (campus admits
    # pending on since>0 list queries; other entities don't).
    n_ok = Network.objects.create(
        status="ok", name="StatusNet-OK", asn=65101,
    )
    n_pending = Network.objects.create(
        status="pending", name="StatusNet-Pending", asn=65102,
    )
    n_deleted = Network.objects.create(
        status="deleted", name="StatusNet-Deleted", asn=65103,
    )
    org_pending = Organization.objects.create(
        status="pending", name="StatusOrg-Pending",
    )
    org_deleted = Organization.objects.create(
        status="deleted", name="StatusOrg-Deleted",
    )
    ix_pending = InternetExchange.objects.create(
        status="pending", name="StatusIX-Pending",
    )
    ix_deleted = InternetExchange.objects.create(
        status="deleted", name="StatusIX-Deleted",
    )
    # STATUS-03 carve-out: campus pending row admitted on since>0.
    campus_pending = Campus.objects.create(
        status="pending", name="StatusCampus-Pending",
    )
    campus_ok = Campus.objects.create(
        status="ok", name="StatusCampus-OK",
    )
    campus_deleted = Campus.objects.create(
        status="deleted", name="StatusCampus-Deleted",
    )


def test_limit_unlimited_001(self):
    # LIMIT-01 / LIMIT-02 seed rows. The bulk Network synthesis lives
    # in the parser (see parseLimit) — these provide the entity-shape
    # anchors so the parser's bulk emitter shares the same field
    # surface as the real upstream rows.
    n_seed = Network.objects.create(
        status="ok", name="LimitNet-Seed", asn=65201,
    )
    org_seed = Organization.objects.create(
        status="ok", name="LimitOrg-Seed",
    )
