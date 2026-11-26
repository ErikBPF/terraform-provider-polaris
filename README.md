# Apache Polaris Terraform Provider

[![Test](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/test.yml/badge.svg)](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/test.yml)
[![Documentation](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/docs.yml/badge.svg)](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/docs.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Typed Terraform provider for Apache Polaris **1.9** IAM. Manages principals,
principal roles, catalog roles, assignments, and catalog/namespace/table grants.
The separate [IAM repository](https://github.com/ErikBPF/terraform-polaris-iam)
applies group-only policy and nested membership; this provider owns API lifecycles.

## Install

Intended Registry address: `ErikBPF/polaris`. **No Registry release exists yet.**
IAM pins a published Git commit and builds a local filesystem mirror for
`0.1.0-dev`; follow its bootstrap instructions before init. Do not assume a
Registry version resolves before the first signed release.

```hcl
terraform {
  required_providers {
    polaris = { source = "ErikBPF/polaris", version = "0.1.0-dev" }
  }
}
provider "polaris" {
  base_url = "https://polaris.example.com"
  realm    = "POLARIS"
  # Supply POLARIS_TOKEN in the environment for external authentication.
}
```

Resource schemas, examples and imports are in [docs/](docs/index.md).
Credentials belong in the environment, never committed configuration.

## Authentication and ownership

`POLARIS_TOKEN` supplies a prefetched bearer token, including Entra tokens when
the server accepts them. The provider does not exchange certificate/federated
assertions with Entra or refresh external tokens. Renew them before a bounded
plan/apply and reacquire before retrying after expiry.

Alternatively, `POLARIS_CLIENT_ID` and `POLARIS_CLIENT_SECRET` use the internal
Polaris token endpoint with `PRINCIPAL_ROLE:ALL`; expiring internal tokens are
refetched. `POLARIS_BASE_URL` and `POLARIS_REALM` select the origin and realm.
TLS verification is always enabled; HTTP is accepted only on loopback for fixtures.

Each resource owns one object, assignment or explicit grant. Reads detect removed
objects; authorization failures fail instead of removing state. Import existing
objects explicitly. Deletes tolerate missing objects, property updates use the
latest entity version, and grants preserve namespace components and other grants.
Writes serialize within one provider instance; use `-parallelism=1` and a locked
backend when configurations share catalog roles.

Principal creation discards generated secrets. Only the client ID enters state;
there is no `client_secret` resource attribute or credential output. Credentials
belong to a separate approved process. The creation-only rotation marker is not
enforcement: server feature settings decide that behavior. Treat state, plans and
debug logs as sensitive regardless. Removing a declaration deletes its registration.

## Development

Requires Go **1.26+**, Python **3.10+**, Terraform **1.13+**, and Docker or Podman
for acceptance. CI pins Go and Terraform toolchains.

```sh
make fmt test build
make docs
git diff --exit-code -- docs examples
make acceptance
```

Unit tests use fake HTTP servers and the race detector. Acceptance starts one
owned loopback Polaris 1.9 container and checks all eight resources, property
updates, no-op, imports, drift repair and destroy. It never takes a shared URL.
FILE/readiness overrides are fixture-only. Azure vending, engine row reads and
external IdP authentication require separate validation.

[RELEASING.md](RELEASING.md) describes signed, reviewer-approved releases.
[UPSTREAM.md](UPSTREAM.md) records provenance and the API baseline.

## License

Apache-2.0. Scaffold history, [LICENSE](LICENSE) and [NOTICE](NOTICE) are preserved.
Implementation is independent; no unlicensed community-provider code is copied.
