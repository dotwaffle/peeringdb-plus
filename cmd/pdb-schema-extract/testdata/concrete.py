from django.db import models
from django_peeringdb.models.abstract import HandleRefModel, AddressModel

class Organization(HandleRefModel, AddressModel):
    name = models.CharField(max_length=255, unique=True, help_text="Organization name")
    aka = models.CharField(max_length=255, blank=True, default="")
    name_long = models.CharField(max_length=255, blank=True, default="")
    website = models.URLField(blank=True, default="")
    notes = models.TextField(blank=True, default="")
    logo = models.CharField(max_length=255, null=True, blank=True)

class Network(HandleRefModel):
    org = models.ForeignKey(Organization, on_delete=models.CASCADE)
    name = models.CharField(max_length=255, unique=True, help_text="Network name")
    aka = models.CharField(max_length=255, blank=True, default="")
    asn = models.PositiveIntegerField(unique=True, help_text="Autonomous System Number")
    info_type = models.CharField(max_length=60, blank=True, default="")
    info_prefixes4 = models.IntegerField(null=True, blank=True)
    info_prefixes6 = models.IntegerField(null=True, blank=True)
    info_traffic = models.CharField(max_length=39, blank=True, default="")
    policy_general = models.CharField(max_length=72, blank=True, default="")
    allow_ixp_update = models.BooleanField(default=False)

class Facility(HandleRefModel, AddressModel):
    org = models.ForeignKey(Organization, on_delete=models.CASCADE)
    name = models.CharField(max_length=255, unique=True, help_text="Facility name")
    website = models.URLField(blank=True, default="")
