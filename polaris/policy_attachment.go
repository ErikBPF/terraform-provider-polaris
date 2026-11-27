package polaris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type attachmentID struct {
	Catalog    string   `json:"catalog"`
	Namespace  []string `json:"namespace"`
	Name       string   `json:"name"`
	TargetType string   `json:"target_type"`
	TargetPath []string `json:"target_path"`
}

func policyAttachmentResource() *schema.Resource {
	r := &schema.Resource{Description: "Manage a direct policy attachment. Parameters are configuration-owned: Polaris cannot return them for drift detection or import.", Schema: map[string]*schema.Schema{
		"catalog": nameSchema("Catalog containing policy and target."), "namespace": namespaceSchema(), "name": nameSchema("Policy name."),
		"target_type": {Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.StringInSlice([]string{"catalog", "namespace", "table-like"}, false), Description: "Attachment target kind."},
		"target_path": {Type: schema.TypeList, Optional: true, ForceNew: true, Elem: &schema.Schema{Type: schema.TypeString, ValidateFunc: validName}, Description: "Target namespace components, followed by table/view name for table-like targets. Empty for catalog."},
		"parameters":  {Type: schema.TypeMap, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}, Description: "Write-only attachment parameters. Import cannot recover these; explicit changes reattach."},
	}}
	identity := func(d *schema.ResourceData) attachmentID {
		p := []string{}
		for _, v := range d.Get("target_path").([]any) {
			p = append(p, v.(string))
		}
		return attachmentID{Catalog: text(d, "catalog"), Namespace: namespace(d), Name: text(d, "name"), TargetType: text(d, "target_type"), TargetPath: p}
	}
	validate := func(id attachmentID) error {
		if err := (assetIdentity{Catalog: id.Catalog, Namespace: id.Namespace, Name: id.Name}).validate("policy"); err != nil {
			return err
		}
		switch id.TargetType {
		case "catalog":
			if len(id.TargetPath) != 0 {
				return fmt.Errorf("catalog target_path must be empty")
			}
		case "namespace":
			if len(id.TargetPath) < 1 {
				return fmt.Errorf("namespace target_path is required")
			}
		case "table-like":
			if len(id.TargetPath) < 2 {
				return fmt.Errorf("table-like target_path requires namespace and object")
			}
		default:
			return fmt.Errorf("invalid attachment target_type")
		}
		for _, p := range id.TargetPath {
			if _, errs := namespaceSchema().Elem.(*schema.Schema).ValidateFunc(p, "target_path"); len(errs) > 0 {
				return errs[0]
			}
		}
		return nil
	}
	path := func(d *schema.ResourceData) string { return assetPath("policy", d) + "/mappings" }
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		id := identity(d)
		if err := validate(id); err != nil {
			return diag.FromErr(err)
		}
		q := url.Values{}
		if id.TargetType != "catalog" {
			parts := id.TargetPath
			if id.TargetType == "table-like" {
				q.Set("target-name", parts[len(parts)-1])
				parts = parts[:len(parts)-1]
			}
			q.Set("namespace", strings.Join(parts, "\x1f"))
		}
		seen := map[string]bool{}
		for {
			var result struct {
				Policies []struct {
					Name      string   `json:"name"`
					Namespace []string `json:"namespace"`
					Inherited *bool    `json:"inherited"`
				} `json:"applicable-policies"`
				Next string `json:"next-page-token"`
			}
			status, err := m.(*client).apiRequest(ctx, http.MethodGet, "/api/catalog/polaris/v1/"+segment(id.Catalog)+"/applicable-policies?"+q.Encode(), nil, &result, 200, 404)
			if err != nil {
				return diag.FromErr(err)
			}
			if status == 404 {
				d.SetId("")
				return nil
			}
			if result.Policies == nil {
				return diag.Errorf("invalid applicable-policies response")
			}
			for _, p := range result.Policies {
				if p.Name == "" || p.Namespace == nil || p.Inherited == nil {
					return diag.Errorf("invalid applicable policy identity")
				}
				if !*p.Inherited && p.Name == id.Name && reflect.DeepEqual(p.Namespace, id.Namespace) {
					return nil
				}
			}
			if result.Next == "" {
				break
			}
			if seen[result.Next] {
				return diag.Errorf("repeated applicable-policy pagination token")
			}
			seen[result.Next] = true
			q.Set("page-token", result.Next)
		}
		d.SetId("")
		return nil
	}
	put := func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		c := m.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		id := identity(d)
		if err := validate(id); err != nil {
			return diag.FromErr(err)
		}
		payload := map[string]any{"target": map[string]any{"type": id.TargetType, "path": id.TargetPath}, "parameters": d.Get("parameters")}
		if _, err := c.apiRequest(ctx, http.MethodPut, path(d), payload, nil, 204); err != nil {
			return diag.FromErr(err)
		}
		d.SetId(encodeJSON(id))
		return r.ReadContext(ctx, d, m)
	}
	r.CreateContext = put
	r.UpdateContext = put
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		c := m.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		if diags := r.ReadContext(ctx, d, m); diags.HasError() {
			return diags
		}
		if d.Id() == "" {
			return nil
		}
		id := identity(d)
		if _, err := c.apiRequest(ctx, http.MethodPost, path(d), map[string]any{"target": map[string]any{"type": id.TargetType, "path": id.TargetPath}}, nil, 204, 404); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	r.Importer = &schema.ResourceImporter{StateContext: func(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
		var id attachmentID
		if err := json.Unmarshal([]byte(d.Id()), &id); err != nil {
			return nil, fmt.Errorf("invalid JSON attachment identity")
		}
		if err := validate(id); err != nil {
			return nil, err
		}
		for k, v := range map[string]any{"catalog": id.Catalog, "namespace": id.Namespace, "name": id.Name, "target_type": id.TargetType, "target_path": id.TargetPath} {
			if err := d.Set(k, v); err != nil {
				return nil, err
			}
		}
		return []*schema.ResourceData{d}, nil
	}}
	return r
}
