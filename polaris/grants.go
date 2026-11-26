package polaris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type grant struct {
	Type      string   `json:"type"`
	Privilege string   `json:"privilege"`
	Namespace []string `json:"namespace,omitempty"`
	Table     string   `json:"tableName,omitempty"`
	View      string   `json:"viewName,omitempty"`
	Policy    string   `json:"policyName,omitempty"`
	Model     string   `json:"semanticModelName,omitempty"`
}

type grantID struct {
	Catalog string `json:"catalog"`
	Role    string `json:"catalog_role"`
	Grant   grant  `json:"grant"`
}

func grantResource(kind string) *schema.Resource {
	field := map[string]string{"table": "table_name", "view": "view_name", "policy": "policy_name", "semantic_model": "semantic_model_name"}[kind]
	apiKind := kind
	if kind == "semantic_model" {
		apiKind = "semantic-model"
	}
	objectName := func(g grant) string {
		switch kind {
		case "table":
			return g.Table
		case "view":
			return g.View
		case "policy":
			return g.Policy
		case "semantic_model":
			return g.Model
		}
		return ""
	}

	attributes := map[string]*schema.Schema{
		"catalog":      nameSchema("Existing catalog name."),
		"catalog_role": nameSchema("Catalog role receiving this privilege; direct identity grants are not supported."),
		"privilege":    {Type: schema.TypeString, Required: true, ForceNew: true, Description: "Polaris 1.9 privilege at this scope.", ValidateFunc: validation.StringInSlice(privileges[kind], false)},
	}
	if kind != "catalog" {
		attributes["namespace"] = &schema.Schema{Type: schema.TypeList, Required: true, ForceNew: true, MinItems: 1, Elem: &schema.Schema{Type: schema.TypeString, ValidateFunc: validation.StringLenBetween(1, 256)}, Description: "Namespace path components; a literal dot remains part of its component."}
	}
	if field != "" {
		attributes[field] = nameSchema("Existing object name.")
	}
	value := func(d *schema.ResourceData) grant {
		g := grant{Type: apiKind, Privilege: text(d, "privilege")}
		if kind != "catalog" {
			for _, v := range d.Get("namespace").([]any) {
				g.Namespace = append(g.Namespace, v.(string))
			}
		}
		switch kind {
		case "table":
			g.Table = text(d, field)
		case "view":
			g.View = text(d, field)
		case "policy":
			g.Policy = text(d, field)
		case "semantic_model":
			g.Model = text(d, field)
		}
		return g
	}
	path := func(d *schema.ResourceData) string {
		return "/catalogs/" + segment(text(d, "catalog")) + "/catalog-roles/" + segment(text(d, "catalog_role")) + "/grants"
	}
	r := &schema.Resource{Schema: attributes, Description: "Manage one explicit " + kind + " privilege for one catalog role. This does not replace other grants or remove inherited access."}
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		var result struct {
			Grants []grant `json:"grants"`
		}
		status, err := meta.(*client).request(ctx, http.MethodGet, path(d), nil, &result, 200, 404)
		if err != nil {
			return diag.FromErr(err)
		}
		if status == 200 {
			if result.Grants == nil {
				return diag.Errorf("invalid Polaris grants response: grants must be an array")
			}
			for _, g := range result.Grants {
				if g.Type == "" || g.Privilege == "" || (g.Type != "catalog" && g.Namespace == nil) || (g.Type == apiKind && field != "" && objectName(g) == "") {
					return diag.Errorf("invalid Polaris grant response: required fields are missing")
				}
			}
			wanted := value(d)
			for _, g := range result.Grants {
				if reflect.DeepEqual(g, wanted) {
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
		if _, err := c.request(ctx, http.MethodPut, path(d), map[string]grant{"grant": value(d)}, nil, 201); err != nil {
			return diag.FromErr(err)
		}
		raw, _ := json.Marshal(grantID{Catalog: text(d, "catalog"), Role: text(d, "catalog_role"), Grant: value(d)})
		d.SetId(string(raw))
		return r.ReadContext(ctx, d, meta)
	}
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
		c := meta.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		if diagnostics := r.ReadContext(ctx, d, meta); diagnostics.HasError() {
			return diagnostics
		}
		if d.Id() == "" {
			return nil
		}
		if _, err := c.request(ctx, http.MethodPost, path(d), map[string]grant{"grant": value(d)}, nil, 201, 404); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	r.Importer = &schema.ResourceImporter{StateContext: func(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
		var id grantID
		if err := json.Unmarshal([]byte(d.Id()), &id); err != nil {
			return nil, fmt.Errorf("grant import ID must contain catalog, catalog_role and grant")
		}
		if id.Catalog == "" || id.Role == "" || id.Grant.Type != apiKind || id.Grant.Privilege == "" || (kind != "catalog" && len(id.Grant.Namespace) == 0) || (field != "" && objectName(id.Grant) == "") {
			return nil, fmt.Errorf("grant import ID does not match this resource scope")
		}
		values := map[string]any{"catalog": id.Catalog, "catalog_role": id.Role, "privilege": id.Grant.Privilege}
		for _, name := range []string{id.Catalog, id.Role} {
			if _, errs := validName(name, "import ID"); len(errs) > 0 {
				return nil, fmt.Errorf("grant import ID contains an invalid name")
			}
		}
		if _, errs := validation.StringInSlice(privileges[kind], false)(id.Grant.Privilege, "privilege"); len(errs) > 0 {
			return nil, fmt.Errorf("grant import ID contains an invalid privilege")
		}
		for _, component := range id.Grant.Namespace {
			if len(component) == 0 || len(component) > 256 {
				return nil, fmt.Errorf("grant import ID contains an invalid namespace component")
			}
		}
		if kind == "catalog" && (len(id.Grant.Namespace) != 0 || id.Grant.Table != "" || id.Grant.View != "" || id.Grant.Policy != "" || id.Grant.Model != "") {
			return nil, fmt.Errorf("catalog grant import ID contains a narrower scope")
		}
		if kind == "namespace" && (id.Grant.Table != "" || id.Grant.View != "" || id.Grant.Policy != "" || id.Grant.Model != "") {
			return nil, fmt.Errorf("namespace grant import ID contains a table")
		}
		if field != "" {
			if _, errs := validName(objectName(id.Grant), field); len(errs) > 0 {
				return nil, fmt.Errorf("grant import ID contains an invalid table name")
			}
		}
		if kind != "catalog" {
			values["namespace"] = id.Grant.Namespace
		}
		if field != "" {
			values[field] = objectName(id.Grant)
		}
		for k, v := range values {
			if err := d.Set(k, v); err != nil {
				return nil, err
			}
		}
		return []*schema.ResourceData{d}, nil
	}}
	return r
}
