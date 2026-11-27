# Source and compatibility baseline

This repository starts from the licensed
[`apache/terraform-provider-polaris`](https://github.com/apache/terraform-provider-polaris)
scaffold and retains its history, Apache-2.0 license and NOTICE. The scaffold
contained no usable implementation. Resource lifecycle code here is independently
authored against the tagged Apache Polaris Management and Catalog API specifications.

- API: [Apache Polaris 1.9.0 specification](https://github.com/apache/polaris/blob/apache-polaris-1.9.0/spec/polaris-management-service.yml).
- Specification SHA256: `e00ae36fb1e502dffd5c57846053005bd144b24828cf043d5f14de83e223f91f`.
- Catalog API: [bundled 1.9.0 specification](https://github.com/apache/polaris/blob/apache-polaris-1.9.0/spec/generated/bundled-polaris-catalog-service.yaml).
- Catalog specification SHA256: `648d356abf3bbfa5c3c308fd012df0aa7b5a7e46ebe01d46d60cc41bed096632`.
- Runtime fixture: `docker.io/apache/polaris:1.9.0`.
- Terraform protocol: **5**, through Plugin SDK v2, matching the LiteLLM pattern.
- First owned release: `0.1.0`; `0.1.0-dev` was the source-build version. Scaffold tags do not define owned releases.

The public `baptistegh/polaris` 0.2.0 provider informed the initial compatibility
assessment but its repository declares no license. Its code is not copied. The
MIT generic REST alternative was not adopted because IAM needs typed drift,
import and independently owned grant semantics.

Compatibility covers 21 resources and seven read-only asset lookups. Engine
commits, Entra objects, OIDC server configuration, nested group policy and cloud
RBAC remain outside this provider. Polaris 1.9 view deletion is guarded by an
explicit catalog override disabling metadata purge, following the tagged
[feature configuration](https://github.com/apache/polaris/blob/apache-polaris-1.9.0/polaris-core/src/main/java/org/apache/polaris/core/config/FeatureConfiguration.java). Add a resource only with an API contract, unit tests and
disposable acceptance coverage. Review specification changes, update privilege
enums, regenerate docs and record state/behavior changes in the changelog.
