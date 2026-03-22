from rest_framework import serializers
from django_peeringdb.models import Organization, Network

class ModelSerializer(serializers.ModelSerializer):
    pass

class OrganizationSerializer(ModelSerializer):
    logo = serializers.CharField(read_only=True, required=False)

    class Meta:
        model = Organization
        fields = ["id", "name", "aka", "website", "notes", "logo", "address1", "city", "country", "latitude", "longitude", "created", "updated", "status"]
        read_only_fields = ["id", "created", "updated"]

class NetworkSerializer(ModelSerializer):
    ix_count = serializers.IntegerField(read_only=True)
    fac_count = serializers.IntegerField(read_only=True)
    allow_ixp_update = serializers.BooleanField(required=False)

    class Meta:
        model = Network
        fields = ["id", "org_id", "name", "asn", "info_type", "info_prefixes4", "info_prefixes6", "info_traffic", "policy_general", "allow_ixp_update", "created", "updated", "status"]
        read_only_fields = ["id", "created", "updated"]

class FacilitySerializer(ModelSerializer):
    class Meta:
        model = Facility
        fields = ["id", "org_id", "name", "website", "created", "updated", "status"]
        read_only_fields = ["id", "created", "updated"]
