// Package polaris manages the Polaris 1.9 IAM management API.
package polaris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"base_url":      {Type: schema.TypeString, Optional: true, DefaultFunc: schema.EnvDefaultFunc("POLARIS_BASE_URL", nil), Description: "Polaris HTTP(S) origin, without an API path."},
				"realm":         {Type: schema.TypeString, Optional: true, DefaultFunc: schema.EnvDefaultFunc("POLARIS_REALM", "POLARIS"), Description: "Polaris realm."},
				"token":         {Type: schema.TypeString, Optional: true, Sensitive: true, DefaultFunc: schema.EnvDefaultFunc("POLARIS_TOKEN", nil), Description: "Prefetched bearer token; external tokens are not refreshed."},
				"client_id":     {Type: schema.TypeString, Optional: true, DefaultFunc: schema.EnvDefaultFunc("POLARIS_CLIENT_ID", nil), Description: "Internal Polaris OAuth client ID; ignored when token is set."},
				"client_secret": {Type: schema.TypeString, Optional: true, Sensitive: true, DefaultFunc: schema.EnvDefaultFunc("POLARIS_CLIENT_SECRET", nil), Description: "Internal Polaris OAuth client secret; ignored when token is set."},
			},
			ResourcesMap: map[string]*schema.Resource{
				"polaris_principal":                 resourcePrincipal(),
				"polaris_principal_role":            resourcePrincipalRole(),
				"polaris_catalog_role":              resourceCatalogRole(),
				"polaris_principal_role_assignment": resourcePrincipalRoleAssignment(),
				"polaris_catalog_role_assignment":   resourceCatalogRoleAssignment(),
				"polaris_catalog_grant":             resourceCatalogGrant(),
				"polaris_namespace_grant":           resourceNamespaceGrant(),
				"polaris_table_grant":               resourceTableGrant(),
			},
		}
		p.ConfigureContextFunc = func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
			c, err := newClient(text(d, "base_url"), text(d, "realm"), text(d, "token"), text(d, "client_id"), text(d, "client_secret"))
			if err != nil {
				return nil, diag.FromErr(err)
			}
			return c, nil
		}
		return p
	}
}

func text(d *schema.ResourceData, key string) string { value, _ := d.Get(key).(string); return value }
func segment(value string) string                    { return url.PathEscape(value) }

func nameSchema(description string) *schema.Schema {
	return &schema.Schema{Type: schema.TypeString, Required: true, ForceNew: true, Description: description, ValidateFunc: validName}
}

func validName(value any, key string) ([]string, []error) {
	warnings, errs := validation.StringLenBetween(1, 256)(value, key)
	name, _ := value.(string)
	if name == "." || name == ".." || strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "system$") {
		errs = append(errs, fmt.Errorf("%s cannot use a traversal segment or reserved system name", key))
	}
	return warnings, errs
}

func composite(parts ...string) string { raw, _ := json.Marshal(parts); return string(raw) }

func importer(keys ...string) *schema.ResourceImporter {
	return &schema.ResourceImporter{StateContext: func(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
		parts := []string{d.Id()}
		if len(keys) > 1 {
			if err := json.Unmarshal([]byte(d.Id()), &parts); err != nil {
				return nil, fmt.Errorf("import ID must be a JSON array with %d nonempty strings", len(keys))
			}
		}
		if len(parts) != len(keys) {
			return nil, fmt.Errorf("import ID requires %d components", len(keys))
		}
		for i, key := range keys {
			if _, errs := validName(parts[i], key); len(errs) != 0 || strings.TrimSpace(parts[i]) == "" {
				return nil, fmt.Errorf("import ID contains an invalid name component")
			}
			if err := d.Set(key, parts[i]); err != nil {
				return nil, err
			}
		}
		return []*schema.ResourceData{d}, nil
	}}
}
