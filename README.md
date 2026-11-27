# Apache Polaris Terraform Provider

[![Test](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/test.yml/badge.svg)](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/test.yml)
[![Documentation](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/docs.yml/badge.svg)](https://github.com/ErikBPF/terraform-provider-polaris/actions/workflows/docs.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Typed Terraform provider for Apache Polaris **1.9**. Manages catalog/storage,
namespace, table/view creation and registration, generic tables, semantic models,
policies/attachments, principals, roles, assignments and scoped grants.
The [platform IAM repository](https://github.com/nstechhub/dataplatform-iam)
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

Each grant refresh reads its catalog role's full explicit-grant inventory; many
grants on one role cause repeated full-list decoding. Responses are limited to
8 MiB. Benchmark large per-table policies before adoption. An oversized inventory
fails refresh and deletion without clearing state; an approved administrator must
reduce/reorganize that role through the Polaris API before Terraform can resume.
No automatic cache or out-of-band revocation is performed by this provider.

Principal creation discards generated secrets. Only the client ID enters state;
there is no `client_secret` resource attribute or credential output. Credentials
belong to a separate approved process. The creation-only rotation marker is not
enforcement: server feature settings decide that behavior. Treat state, plans and
debug logs as sensitive regardless. Removing a declaration deletes its registration.

## Asset ownership and safe deletion

Asset resources reconcile only declared property keys and preserve engine-added
keys. Creation schema, partition/sort order, view version, location and initial
registration metadata are creation-only inputs. Current metadata is computed;
ordinary engine commits do not reset it. Omit creation-only inputs when importing
an existing table/view; import cannot reconstruct their original creation values.
Only one Terraform resource may own a given remote object.

`polaris_catalog.storage_config_json` accepts tagged 1.9 StorageConfigInfo JSON
(FILE, AZURE, S3 or GCS). Azure accounts, containers, identity and RBAC remain Azure
IaC resources. Catalog EXTERNAL connection configuration is experimental,
creation-only and potentially secret-bearing in state; it requires compatible
server configuration and separate federation validation.

Table deletion always requests `purgeRequested=false`. **View deletion requires
catalog property `polaris.config.purge-view-metadata-on-drop = "false"`.** Polaris
1.9 defaults to purging view metadata and exposes no per-request purge switch.
The provider checks that explicit catalog override before dropping a view and
refuses otherwise; configure it through the catalog owner before destroy. Catalog
and namespace deletion refuses nonempty parents. Policy deletion does not detach
other mappings. Generic table fields are immutable because the API has no update.

Registration requires an existing Iceberg metadata JSON file, never a raw Parquet
or CSV directory. It refuses occupied names; configured properties are committed
following registration, with the registration ID retained if that commit fails.
Policy attachment parameters cannot be read back by Polaris 1.9: explicit changes
are applied, but drift detection/import cannot recover them. Direct attachment
existence is checked separately from inherited policy applicability.

Seven read-only data sources return durable metadata and exclude vended bearer
and storage credentials. Plans/state still contain user metadata and configuration;
protect them. Server OIDC/JDBC/feature flags, cloud credentials and engine schema,
snapshot/data evolution are outside these resource lifecycles.

## Development

Requires Go **1.26+**, Python **3.10+**, Terraform **1.13+**, and Docker or Podman
for acceptance. CI pins Go and Terraform toolchains.

```sh
make fmt test build
make docs
git diff --exit-code -- docs examples
make acceptance
make acceptance-minikube KUBE_CONTEXT=<explicit-minikube-context>
```

Unit tests use fake HTTP servers and the race detector. Acceptance starts one
owned loopback Polaris 1.9 container and checks all 21 resources, property
updates, no-op, imports, drift repair and destroy. It never takes a shared URL.
The minikube runner checks node labels, creates one UUID namespace, uses a Secret
and loopback port forwarding, then removes only that namespace. FILE/readiness
and semantic-model overrides are fixture-only. Azure vending, engine row reads and
external IdP authentication require separate validation.

[RELEASING.md](RELEASING.md) describes signed, reviewer-approved releases.
[UPSTREAM.md](UPSTREAM.md) records provenance and the API baseline.

## License

Apache-2.0. Scaffold history, [LICENSE](LICENSE) and [NOTICE](NOTICE) are preserved.
Implementation is independent; no unlicensed community-provider code is copied.
