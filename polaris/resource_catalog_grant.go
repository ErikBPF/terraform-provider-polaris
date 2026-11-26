package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceCatalogGrant() *schema.Resource { return grantResource("catalog") }
