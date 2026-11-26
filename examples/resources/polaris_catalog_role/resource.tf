resource "polaris_catalog_role" "reader" {
  catalog = "warehouse"
  name    = "iam_analysts"
}
