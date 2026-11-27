package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceCatalogRole() *schema.Resource { return entityResource("catalogRole") }
