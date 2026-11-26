resource "polaris_namespace_grant" "access" {
  catalog      = "warehouse"
  catalog_role = "iam_analysts"
  namespace    = ["sales", "region"]
  privilege    = "NAMESPACE_READ_PROPERTIES"
}
