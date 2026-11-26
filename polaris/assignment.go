package polaris

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func assignmentResource(catalog bool) *schema.Resource {
	keys := []string{"principal", "principal_role"}
	if catalog {
		keys = []string{"catalog", "principal_role", "catalog_role"}
	}
	attributes := map[string]*schema.Schema{}
	for _, key := range keys {
		attributes[key] = nameSchema("Existing Polaris " + key + " name.")
	}
	collection := func(d *schema.ResourceData) string {
		if catalog {
			return "/principal-roles/" + segment(text(d, "principal_role")) + "/catalog-roles/" + segment(text(d, "catalog"))
		}
		return "/principals/" + segment(text(d, "principal")) + "/principal-roles"
	}
	role := func(d *schema.ResourceData) string {
		if catalog {
			return text(d, "catalog_role")
		}
		return text(d, "principal_role")
	}
	r := &schema.Resource{Schema: attributes, Importer: importer(keys...), Description: "Manage one Polaris role assignment. Other assignments remain independently owned."}
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		var result struct {
			Roles []entity `json:"roles"`
		}
		status, err := meta.(*client).request(ctx, http.MethodGet, collection(d), nil, &result, 200, 404)
		if err != nil {
			return diag.FromErr(err)
		}
		if status == 200 {
			if result.Roles == nil {
				return diag.Errorf("invalid Polaris assignment response: roles must be an array")
			}
			for _, value := range result.Roles {
				if value.Name == "" {
					return diag.Errorf("invalid Polaris assignment response: role name is missing")
				}
			}
			for _, value := range result.Roles {
				if value.Name == role(d) {
					return nil
				}
			}
		}
		d.SetId("")
		return nil
	}
	r.CreateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		key := "principalRole"
		if catalog {
			key = "catalogRole"
		}
		if _, err := c.request(ctx, http.MethodPut, collection(d), map[string]any{key: map[string]string{"name": role(d)}}, nil, 201); err != nil {
			return diag.FromErr(err)
		}
		parts := []string{}
		for _, key := range keys {
			parts = append(parts, text(d, key))
		}
		d.SetId(composite(parts...))
		return r.ReadContext(ctx, d, meta)
	}
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		if _, err := c.request(ctx, http.MethodDelete, collection(d)+"/"+segment(role(d)), nil, nil, 204, 404); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	return r
}
