resource "polaris_view_grant" "example" {
  catalog      = "warehouse"
  namespace    = ["sales"]
  catalog_role = "iam_analysts"
  view_name    = "chosen"
  privilege    = "VIEW_READ_PROPERTIES"
}
