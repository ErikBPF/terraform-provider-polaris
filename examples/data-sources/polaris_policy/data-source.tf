data "polaris_policy" "example" {
  catalog   = "warehouse"
  namespace = ["sales"]
  name      = "chosen"
}
