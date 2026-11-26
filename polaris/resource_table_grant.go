package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceTableGrant() *schema.Resource { return grantResource("table") }
