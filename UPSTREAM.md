# Source and compatibility baseline

This repository starts from the licensed
[`apache/terraform-provider-polaris`](https://github.com/apache/terraform-provider-polaris)
scaffold and retains its history, Apache-2.0 license and NOTICE. The scaffold
contained no usable implementation. Resource lifecycle code here is independently
authored against the Apache Polaris management API specification.

- API: [Apache Polaris 1.9.0 specification](https://github.com/apache/polaris/blob/apache-polaris-1.9.0/spec/polaris-management-service.yml).
- Specification SHA256: `e00ae36fb1e502dffd5c57846053005bd144b24828cf043d5f14de83e223f91f`.
- Runtime fixture: `docker.io/apache/polaris:1.9.0`.
- Terraform protocol: **5**, through Plugin SDK v2, matching the LiteLLM pattern.
- First owned version: `0.1.0-dev`; scaffold tags do not define owned releases.

The public `baptistegh/polaris` 0.2.0 provider informed the initial compatibility
assessment but its repository declares no license. Its code is not copied. The
MIT generic REST alternative was not adopted because IAM needs typed drift,
import and independently owned grant semantics.

Compatibility is limited to IAM's eight resource types. Dataset lifecycle, Entra
objects, OIDC server configuration, nested group policy and storage RBAC remain
outside this provider. Add a resource only with an API contract, unit tests and
disposable acceptance coverage. Review specification changes, update privilege
enums, regenerate docs and record state/behavior changes in the changelog.
