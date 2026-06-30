# Synthetic stand-in for peeringdb_server/models.py.
#
# The concrete server models subclass the django-peeringdb abstract *Base
# classes (see abstract.py) and add the foreign keys plus server-specific fields
# (allow_ixp_update, ixp_update_exclude). This is the source the extractor reads
# for the concrete model shape; django-peeringdb's own concrete.py is not used.
from django.db import models
from django_peeringdb.models.abstract import OrganizationBase, NetworkBase


class Organization(OrganizationBase, StripFieldMixin):
    logo = models.FileField(null=True, blank=True)


class Network(NetworkBase, StripFieldMixin):
    org = models.ForeignKey(
        Organization,
        on_delete=models.CASCADE,
        null=True,
        blank=True,
        related_name="net_set",
    )
    allow_ixp_update = models.BooleanField(default=False)
    ixp_update_exclude = models.JSONField(default=list, blank=True)
