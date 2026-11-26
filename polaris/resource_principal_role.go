package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourcePrincipalRole() *schema.Resource { return entityResource("principalRole") }
