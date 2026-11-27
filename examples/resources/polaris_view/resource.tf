resource "polaris_view" "example" {
  catalog           = "warehouse"
  namespace         = ["sales"]
  name              = "chosen"
  schema_json       = jsonencode({ type = "struct", "schema-id" = 0, fields = [{ id = 1, name = "id", required = false, type = "long" }] })
  view_version_json = jsonencode({ "version-id" = 1, "timestamp-ms" = 1700000000000, "schema-id" = 0, summary = {}, representations = [{ type = "sql", sql = "select 1 as id", dialect = "spark" }], "default-namespace" = ["sales"] })
}
