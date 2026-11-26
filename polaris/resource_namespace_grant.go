package polaris

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceNamespaceGrant() *schema.Resource { return grantResource("namespace") }
