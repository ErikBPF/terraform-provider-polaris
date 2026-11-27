# Changelog

## 0.1.0

First owned release for Apache Polaris 1.9.0, based on the licensed Apache scaffold.

- Manage 21 resource types: principals, roles, assignments and scoped grants;
  catalog/storage configuration and namespaces; Iceberg table/view creation and
  registration; generic tables, semantic models, policies and direct attachments.
- Add seven read-only asset lookups, import examples and generated documentation.
- Preserve engine-owned properties and moving metadata pointers. Table deletion
  disables purge; view deletion requires the explicit catalog override disabling
  metadata purge. Nonempty parent deletion and occupied registration names fail.
- Discard generated principal secrets, verify TLS, refuse redirects and preserve
  state on authorization failures or malformed responses.
- Validate typed API behavior through unit/race checks and disposable Polaris
  acceptance, including isolated minikube fixtures.
- Publish signed multi-platform archives with a protocol-5 Registry manifest
  through the reviewer-protected production-release environment.

The development source-build version was 0.1.0-dev. Resource identities and schema
versions remain unchanged in 0.1.0; review dependency locks when adopting the release.
External IdP authentication, cloud credential vending and engine row I/O require
separate environment validation. Policy attachment parameter drift/import cannot
be recovered through the Polaris 1.9 API.
