resource "polaris_table" "example" {
  catalog     = "warehouse"
  namespace   = ["sales"]
  name        = "chosen"
  schema_json = jsonencode({ type = "struct", "schema-id" = 0, fields = [{ id = 1, name = "id", required = false, type = "long" }] })
}
