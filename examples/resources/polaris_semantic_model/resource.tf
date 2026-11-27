resource "polaris_semantic_model" "example" {
  catalog       = "warehouse"
  namespace     = ["sales"]
  name          = "chosen"
  document_json = jsonencode({ version = "0.2.0.dev0", semantic_model = jsonencode({ name = "sales", datasets = [] }) })
}
