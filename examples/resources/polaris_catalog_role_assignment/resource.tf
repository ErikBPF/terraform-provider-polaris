resource "polaris_catalog_role_assignment" "binding" {
  catalog        = "warehouse"
  catalog_role   = "iam_analysts"
  principal_role = "analysts"
}
