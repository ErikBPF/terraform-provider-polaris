data "polaris_table" "example" {
  catalog   = "warehouse"
  namespace = ["sales"]
  name      = "chosen"
}
