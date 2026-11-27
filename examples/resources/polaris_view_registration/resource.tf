resource "polaris_view_registration" "example" {
  catalog           = "warehouse"
  namespace         = ["sales"]
  name              = "chosen"
  metadata_location = "file:///tmp/warehouse/sales/chosen/metadata/existing.metadata.json"
}
