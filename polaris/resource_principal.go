package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourcePrincipal() *schema.Resource { return entityResource("principal") }
