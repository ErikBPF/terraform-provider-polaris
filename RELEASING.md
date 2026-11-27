# Releasing

Use this repository's own semantic versions, starting with `0.1.0`.
`0.1.0-dev` was the pre-release source-build mirror version consumed by IAM.

## One-time setup

1. Use the maintainer signing key already registered for the `ErikBPF` namespace.
2. Verify [docs/release-signing-key.asc](docs/release-signing-key.asc) has fingerprint
   `F3E389BAE314FA1CEE23C06E64588A337187C497`.
3. Configure `GPG_PRIVATE_KEY`, `GPG_PASSPHRASE` and variable
   `RELEASE_SIGNING_KEY_FINGERPRINT` only in the protected `production-release`
   environment. Do not duplicate signing keys as repository secrets: those
   would be usable by jobs that do not pass the environment approval gate.
4. Confirm the IaC-managed `production-release` environment requires ErikBPF
   approval and disables admin bypass. Its name alone does not create a gate.
5. The provider registration is already created. Confirm each signed release is
   indexed; resync through Registry settings if a release is not discovered.

The key is shared with the maintainer's other providers. Signing values are resolved
from the existing encrypted store into the protected environment; no private key,
passphrase, state, plan or debug log belongs in source. Keep prior public signing
keys and immutable release assets available for existing consumers.

## Before tagging

1. Update `CHANGELOG.md` with migration notes and user-visible behavior.
2. Run `make fmt test build docs`; require clean generated files.
3. Run `make acceptance` against its disposable container.
4. Inspect a GoReleaser snapshot, archive names, manifest and checksums.
5. Merge the reviewed provider PR before creating a new semantic-version tag.

The tag workflow reruns acceptance, verifies the commit belongs to `main`, waits
for environment approval, verifies the configured signing fingerprint and
publishes signed archives/checksums with a protocol-5 manifest. Tags are an
explicit maintainer action; ordinary CI cannot publish releases.

## Verify and adopt

Verify the checksum signature against the registered public key and archive
checksums. Confirm Registry resolves the exact new `ErikBPF/polaris` version;
then replace IAM's source-build pin with that release and review its dependency
lockfile. Run IAM integration before applying to dev.

Never replace tags or assets. Fix forward with a new version and keep consumers
pinned to their previous verified artifact if verification fails.
