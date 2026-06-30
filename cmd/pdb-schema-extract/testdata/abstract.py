# Synthetic stand-in for django-peeringdb's models/abstract.py.
#
# Mirrors the real structure the extractor relies on: shared roots
# (HandleRefModel, AddressModel) and per-entity abstract *Base classes that the
# concrete server models in models.py subclass. Kept deliberately small — just
# enough to exercise inheritance resolution, the ASNField custom constructor,
# and multi-line field definitions.
from django.db import models


class HandleRefModel(models.Model):
    status = models.CharField(max_length=255, default="ok")
    created = models.DateTimeField(help_text="Created timestamp")
    updated = models.DateTimeField(help_text="Updated timestamp")

    class Meta:
        abstract = True


class AddressModel(models.Model):
    city = models.CharField(max_length=255, blank=True, default="")
    country = models.CharField(max_length=255, blank=True, default="")
    latitude = models.DecimalField(max_digits=9, decimal_places=6, null=True, blank=True)
    longitude = models.DecimalField(max_digits=9, decimal_places=6, null=True, blank=True)

    class Meta:
        abstract = True


class OrganizationBase(HandleRefModel, AddressModel):
    name = models.CharField(max_length=255, unique=True, help_text="Organization name")
    aka = models.CharField(max_length=255, blank=True, default="")
    website = models.URLField(blank=True, default="")

    class Meta:
        abstract = True


class NetworkBase(HandleRefModel):
    name = models.CharField(max_length=255, unique=True, help_text="Network name")
    aka = models.CharField(max_length=255, blank=True, default="")
    asn = ASNField(
        unique=True,
        help_text="Autonomous System Number",
    )
    irr_as_set = models.CharField(max_length=255, blank=True, default="")
    info_prefixes4 = models.PositiveIntegerField(null=True, blank=True)

    class Meta:
        abstract = True
