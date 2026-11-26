# Changelog

## Unreleased

- Add eight typed Polaris 1.9 IAM resources with explicit imports and drift detection.
- Keep generated principal secrets out of Terraform state.
- Add unit, generated-documentation and disposable acceptance CI.
- Prepare signed releases with a reviewer-backed `production-release` environment.

- Add catalog/storage and namespace lifecycle, Iceberg table/view creation and registration, generic tables, semantic models, policies and attachments.
- Add view/policy/model grants, seven durable metadata lookups and isolated minikube acceptance.
- Guard view drops against Polaris default metadata purging; preserve engine-owned properties and current metadata pointers.

No release has been published. IAM uses a pinned source build.
