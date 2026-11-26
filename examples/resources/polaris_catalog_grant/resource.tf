resource "polaris_catalog_grant" "access" {
  catalog      = "warehouse"
  catalog_role = "iam_analysts"
  privilege    = "CATALOG_READ_CONFIG"
}
