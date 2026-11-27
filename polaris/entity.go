package polaris

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type entity struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	ClientID   string            `json:"clientId,omitempty"`
	Version    int64             `json:"entityVersion,omitempty"`
}

func entityResource(kind string) *schema.Resource {
	attributes := map[string]*schema.Schema{
		"name":       nameSchema("Stable Polaris object name."),
		"properties": {Type: schema.TypeMap, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}, Description: "String properties owned by this resource."},
	}
	keys := []string{"name"}
	if kind == "catalogRole" {
		attributes["catalog"] = nameSchema("Existing catalog containing this role.")
		keys = []string{"catalog", "name"}
	}
	if kind == "principal" {
		attributes["client_id"] = &schema.Schema{Type: schema.TypeString, Computed: true, Description: "Generated client ID. The generated secret is discarded and never enters state."}
		attributes["credential_rotation_required"] = &schema.Schema{Type: schema.TypeBool, Optional: true, Default: false, ForceNew: true, Description: "Creation-only credential rotation marker; enforcement depends on server settings."}
	}
	collection := func(d *schema.ResourceData) string {
		switch kind {
		case "principal":
			return "/principals"
		case "principalRole":
			return "/principal-roles"
		default:
			return "/catalogs/" + segment(text(d, "catalog")) + "/catalog-roles"
		}
	}
	path := func(d *schema.ResourceData) string { return collection(d) + "/" + segment(text(d, "name")) }
	set := func(d *schema.ResourceData, value entity) diag.Diagnostics {
		if value.Name == "" || value.Name != text(d, "name") {
			return diag.Errorf("invalid Polaris entity response: expected the requested object name")
		}
		if err := d.Set("name", value.Name); err != nil {
			return diag.FromErr(err)
		}
		if err := d.Set("properties", value.Properties); err != nil {
			return diag.FromErr(err)
		}
		if kind == "principal" {
			if err := d.Set("client_id", value.ClientID); err != nil {
				return diag.FromErr(err)
			}
		}
		return nil
	}
	r := &schema.Resource{Schema: attributes, Importer: importer(keys...), Description: "Manage a Polaris " + kind + ". Objects are never silently adopted; import existing names before applying."}
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		var value entity
		status, err := meta.(*client).request(ctx, http.MethodGet, path(d), nil, &value, 200, 404)
		if err != nil {
			return diag.FromErr(err)
		}
		if status == 404 {
			d.SetId("")
			return nil
		}
		return set(d, value)
	}
	r.CreateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		properties := map[string]string{}
		for k, v := range d.Get("properties").(map[string]any) {
			properties[k] = v.(string)
		}
		payload := map[string]any{kind: entity{Name: text(d, "name"), Properties: properties}}
		if kind == "principal" {
			payload["credentialRotationRequired"] = d.Get("credential_rotation_required")
		}
		if _, err := c.request(ctx, http.MethodPost, collection(d), payload, nil, 201); err != nil {
			return diag.FromErr(err)
		}
		if kind == "catalogRole" {
			d.SetId(composite(text(d, "catalog"), text(d, "name")))
		} else {
			d.SetId(text(d, "name"))
		}
		return r.ReadContext(ctx, d, meta)
	}
	r.UpdateContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		var current entity
		if _, err := c.request(ctx, http.MethodGet, path(d), nil, &current, 200); err != nil {
			return diag.FromErr(err)
		}
		payload := map[string]any{"currentEntityVersion": current.Version, "properties": d.Get("properties")}
		if _, err := c.request(ctx, http.MethodPut, path(d), payload, nil, 200); err != nil {
			return diag.FromErr(err)
		}
		return r.ReadContext(ctx, d, meta)
	}
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		if _, err := c.request(ctx, http.MethodDelete, path(d), nil, nil, 204, 404); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	return r
}
