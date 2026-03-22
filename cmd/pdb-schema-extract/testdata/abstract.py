from django.db import models

class HandleRefModel(models.Model):
    status = models.CharField(max_length=255, default="ok")
    created = models.DateTimeField(help_text="Created timestamp")
    updated = models.DateTimeField(help_text="Updated timestamp")

    class Meta:
        abstract = True

class AddressModel(models.Model):
    address1 = models.CharField(max_length=255, blank=True, default="")
    address2 = models.CharField(max_length=255, blank=True, default="")
    city = models.CharField(max_length=255, blank=True, default="")
    state = models.CharField(max_length=255, blank=True, default="")
    country = models.CharField(max_length=255, blank=True, default="")
    zipcode = models.CharField(max_length=48, blank=True, default="")
    suite = models.CharField(max_length=255, blank=True, default="")
    floor = models.CharField(max_length=255, blank=True, default="")
    latitude = models.DecimalField(max_digits=9, decimal_places=6, null=True, blank=True)
    longitude = models.DecimalField(max_digits=9, decimal_places=6, null=True, blank=True)

    class Meta:
        abstract = True

class SocialMediaMixin(models.Model):
    social_media = models.JSONField(default=list, blank=True)

    class Meta:
        abstract = True
