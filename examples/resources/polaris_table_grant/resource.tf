resource "polaris_table_grant" "access" {
  catalog      = "warehouse"
  catalog_role = "iam_analysts"
  namespace    = ["sales", "region"]
  table_name   = "orders"
  privilege    = "TABLE_READ_DATA"
}
