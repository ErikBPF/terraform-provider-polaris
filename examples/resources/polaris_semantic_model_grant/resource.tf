resource "polaris_semantic_model_grant" "example" {
  catalog             = "warehouse"
  namespace           = ["sales"]
  catalog_role        = "iam_analysts"
  semantic_model_name = "chosen"
  privilege           = "SEMANTIC_MODEL_READ"
}
