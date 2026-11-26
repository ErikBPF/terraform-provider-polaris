resource "polaris_principal" "user" {
  name                         = "existing-entra-object-id"
  properties                   = { identity_type = "user" }
  credential_rotation_required = true
  lifecycle { ignore_changes = [credential_rotation_required] }
}
