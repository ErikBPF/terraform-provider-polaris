resource "polaris_policy_grant" "example" {
  catalog      = "warehouse"
  namespace    = ["sales"]
  catalog_role = "iam_analysts"
  policy_name  = "chosen"
  privilege    = "POLICY_READ"
}
