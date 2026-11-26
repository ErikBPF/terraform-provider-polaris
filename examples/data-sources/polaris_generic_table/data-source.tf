data "polaris_generic_table" "example" {
  catalog   = "warehouse"
  namespace = ["sales"]
  name      = "chosen"
}
