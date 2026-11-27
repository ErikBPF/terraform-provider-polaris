resource "polaris_namespace" "example" {
  catalog    = "warehouse"
  namespace  = ["sales"]
  properties = { owner = "platform" }
}
