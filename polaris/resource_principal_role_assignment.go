package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourcePrincipalRoleAssignment() *schema.Resource { return assignmentResource(false) }
