package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceCatalogRoleAssignment() *schema.Resource { return assignmentResource(true) }
