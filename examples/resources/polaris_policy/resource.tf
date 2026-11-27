resource "polaris_policy" "example" {
  catalog   = "warehouse"
  namespace = ["sales"]
  name      = "chosen"
  type      = "system.data-compaction"
  content   = jsonencode({ version = "2025-02-03", enable = true, config = { target_file_size_bytes = 134217728 } })
}
