resource "polaris_generic_table" "example" {
  catalog       = "warehouse"
  namespace     = ["sales"]
  name          = "chosen"
  format        = "csv"
  base_location = "file:///tmp/warehouse/sales/chosen"
}
