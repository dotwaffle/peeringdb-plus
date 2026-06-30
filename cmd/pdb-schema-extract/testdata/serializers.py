# Synthetic stand-in for peeringdb_server/serializers.py.
#
# Exercises the DRF serializer-introspection paths the extractor depends on:
#   - Meta.fields as the authoritative output set,
#   - Meta.related_fields (nested object + reverse <x>_set) exclusion,
#   - a PrimaryKeyRelatedField FK id (queryset + source= remap),
#   - a SerializerMethodField typed from its get_<name> return annotation.
from rest_framework import serializers

from peeringdb_server.models import Network, Organization


class ModelSerializer(serializers.ModelSerializer):
    pass


class OrganizationSerializer(ModelSerializer):
    class Meta:
        model = Organization
        fields = ["id", "name", "aka", "website", "created", "updated", "status"]
        read_only_fields = ["id", "created", "updated"]


class NetworkSerializer(ModelSerializer):
    org_id = serializers.PrimaryKeyRelatedField(
        queryset=Organization.objects.all(),
        source="org",
    )
    ix_count = serializers.SerializerMethodField()

    def get_ix_count(self, inst) -> int:
        return 0

    class Meta:
        model = Network
        fields = [
            "id",
            "org_id",
            "org",
            "name",
            "aka",
            "asn",
            "irr_as_set",
            "info_prefixes4",
            "allow_ixp_update",
            "ixp_update_exclude",
            "ix_count",
            "poc_set",
            "created",
            "updated",
            "status",
        ]
        related_fields = ["org", "poc_set"]
        read_only_fields = ["id", "created", "updated"]
