resource "polaris_principal_role_assignment" "member" {
  principal      = "existing-entra-object-id"
  principal_role = "analysts"
}
