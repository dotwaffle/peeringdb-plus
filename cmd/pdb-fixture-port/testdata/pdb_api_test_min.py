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
