resource "polaris_catalog" "example" {
  name                = "warehouse"
  properties          = { "default-base-location" = "file:///tmp/warehouse", "polaris.config.purge-view-metadata-on-drop" = "false" }
  storage_config_json = jsonencode({ storageType = "FILE", allowedLocations = ["file:///tmp/warehouse"] })
}
