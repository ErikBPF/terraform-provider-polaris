package polaris

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/structure"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// These resources own declared properties, not engine-added properties or moving metadata pointers.
func assetResource(kind string) *schema.Resource {
	a := map[string]*schema.Schema{"name": nameSchema("Stable object name. Import existing names before applying."), "properties": {Type: schema.TypeMap, Optional: true, Elem: &schema.Schema{Type: schema.TypeString}, Description: "Only these property keys are reconciled; engine-added keys are preserved."}, "metadata_json": {Type: schema.TypeString, Computed: true, Sensitive: true, Description: "Current object metadata, excluding response credential-vending configuration."}}
	if kind != "catalog" {
		a["catalog"] = nameSchema("Containing catalog.")
		a["namespace"] = namespaceSchema()
	}
	jsonField := func(desc string, immutable bool) *schema.Schema {
		return &schema.Schema{Type: schema.TypeString, Optional: true, ForceNew: immutable, Description: desc, ValidateFunc: jsonObject, StateFunc: func(v any) string { s, _ := structure.NormalizeJsonString(v); return s }, DiffSuppressFunc: func(_, old, new string, _ *schema.ResourceData) bool {
			a, e := structure.NormalizeJsonString(old)
			b, f := structure.NormalizeJsonString(new)
			return e == nil && f == nil && a == b
		}}
	}
	strField := func(desc string, immutable bool) *schema.Schema {
		return &schema.Schema{Type: schema.TypeString, Optional: true, ForceNew: immutable, Description: desc}
	}
	switch kind {
	case "catalog":
		a["type"] = &schema.Schema{Type: schema.TypeString, Optional: true, Default: "INTERNAL", ForceNew: true, ValidateFunc: validation.StringInSlice([]string{"INTERNAL", "EXTERNAL"}, false), Description: "Catalog type. External federation requires server configuration."}
		a["storage_config_json"] = jsonField("StorageConfigInfo JSON: storageType, allowedLocations and cloud-specific fields. Does not create cloud resources.", false)
		a["connection_config_json"] = jsonField("Experimental EXTERNAL catalog ConnectionConfigInfo. May contain credentials in state.", true)
		a["connection_config_json"].Sensitive = true
	case "namespace":
		delete(a, "name")
		a["location"] = strField("Optional creation-only namespace location; server defaults remain unmanaged.", true)
	case "table", "table_registration", "view", "view_registration":
		a["current_metadata_location"] = &schema.Schema{Type: schema.TypeString, Computed: true, Description: "Current engine-owned metadata pointer; never used to reset registration."}
		a["location"] = strField("Creation-only object location. Omit when importing.", true)
		if strings.HasSuffix(kind, "registration") {
			delete(a, "location")
			a["metadata_location"] = strField("Initial existing Iceberg metadata JSON file, not a raw data directory. Required for creation; omit when importing.", true)
			a["metadata_location"].ValidateFunc = validation.StringIsNotEmpty
		} else {
			a["schema_json"] = jsonField("Creation-only Iceberg schema JSON. Required to create; omit when importing.", true)
			if kind == "table" {
				a["partition_spec_json"] = jsonField("Creation-only Iceberg partition spec.", true)
				a["write_order_json"] = jsonField("Creation-only Iceberg sort order.", true)
			} else {
				a["view_version_json"] = jsonField("Creation-only Iceberg view version JSON. Required to create; omit when importing.", true)
			}
		}
	case "generic_table":
		a["format"] = &schema.Schema{Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.StringIsNotEmpty, Description: "Generic table format, such as delta or csv."}
		a["base_location"] = strField("Creation-only base location. Drop unregisters without deleting data.", true)
		a["doc"] = strField("Creation-only description.", true)
		a["properties"].ForceNew = true
	case "semantic_model":
		delete(a, "properties")
		a["document_json"] = jsonField("Ossie document object with version and semantic_model (a JSON object serialized as a string).", false)
		a["document_json"].Required = true
		a["document_json"].Optional = false
	case "policy":
		delete(a, "properties")
		a["type"] = &schema.Schema{Type: schema.TypeString, Required: true, ForceNew: true, ValidateFunc: validation.StringIsNotEmpty, Description: "Server-supported maintenance policy type."}
		a["description"] = strField("Policy description.", false)
		a["content"] = strField("Policy content, validated by Polaris for its policy type.", false)
	}
	r := &schema.Resource{Schema: a, Description: "Manage a Polaris " + kind + ". Deletion unregisters without purging object data. Nonempty parent deletion is refused by Polaris."}
	if strings.HasPrefix(kind, "view") {
		r.Description += " Safe drop requires catalog property polaris.config.purge-view-metadata-on-drop=false; otherwise deletion is refused."
	}
	read := func(ctx context.Context, d *schema.ResourceData, c *client) (map[string]any, int, error) {
		var v map[string]any
		status, err := c.apiRequest(ctx, http.MethodGet, assetPath(kind, d), nil, &v, 200, 404)
		if err == nil && status == 200 {
			err = validateAsset(kind, d, v)
		}
		return v, status, err
	}
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		v, status, err := read(ctx, d, m.(*client))
		if err != nil {
			return diag.FromErr(err)
		}
		if status == 404 {
			d.SetId("")
			return nil
		}
		obj := assetObject(kind, v)
		values := map[string]any{"metadata_json": encodeJSON(durableMetadata(kind, obj))}
		if a["properties"] != nil {
			props, _ := obj["properties"].(map[string]any)
			owned := map[string]any{}
			for key := range d.Get("properties").(map[string]any) {
				if val, ok := props[key]; ok {
					owned[key] = val
				}
			}
			values["properties"] = owned
		}
		switch kind {
		case "catalog":
			values["type"] = obj["type"]
			if text(d, "storage_config_json") != "" {
				var configured map[string]any
				json.Unmarshal([]byte(text(d, "storage_config_json")), &configured)
				values["storage_config_json"] = encodeJSON(projectJSON(configured, objectMap(obj["storageConfigInfo"])))
			}
		case "table", "table_registration", "view", "view_registration":
			values["current_metadata_location"] = v["metadata-location"]
		case "generic_table":
			values["format"] = obj["format"]
			values["base_location"] = obj["base-location"]
			values["doc"] = obj["doc"]
		case "semantic_model":
			values["document_json"] = encodeJSON(v["document"])
		case "policy":
			values["type"] = obj["policy-type"]
			values["description"] = obj["description"]
			values["content"] = obj["content"]
		}
		for k, v := range values {
			if v == nil {
				v = ""
			}
			if err := d.Set(k, v); err != nil {
				return diag.FromErr(err)
			}
		}
		return nil
	}
	r.CreateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		c := m.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		payload := map[string]any{"name": text(d, "name")}
		if a["properties"] != nil {
			payload["properties"] = d.Get("properties")
		}
		collection := assetCollection(kind, d)
		status := 200
		switch kind {
		case "catalog":
			payload["type"] = text(d, "type")
			if err := addJSON(payload, "storageConfigInfo", text(d, "storage_config_json")); err != nil {
				return diag.FromErr(err)
			}
			if err := addJSON(payload, "connectionConfigInfo", text(d, "connection_config_json")); err != nil {
				return diag.FromErr(err)
			}
			payload = map[string]any{"catalog": payload}
			status = 201
		case "namespace":
			delete(payload, "name")
			payload["namespace"] = namespace(d)
			if text(d, "location") != "" {
				props := stringMap(d.Get("properties"))
				props["location"] = text(d, "location")
				payload["properties"] = props
			}
		case "table_registration", "view_registration":
			if text(d, "metadata_location") == "" {
				return diag.Errorf("metadata_location is required to register an object")
			}
			delete(payload, "properties")
			payload["metadata-location"] = text(d, "metadata_location")
			if kind == "table_registration" {
				payload["overwrite"] = false
			}
		case "table", "view":
			if text(d, "schema_json") == "" {
				return diag.Errorf("schema_json is required to create an object")
			}
			for field, key := range map[string]string{"schema_json": "schema", "partition_spec_json": "partition-spec", "write_order_json": "write-order", "view_version_json": "view-version"} {
				if a[field] != nil {
					if err := addJSON(payload, key, text(d, field)); err != nil {
						return diag.FromErr(err)
					}
				}
			}
			if kind == "view" && text(d, "view_version_json") == "" {
				return diag.Errorf("view_version_json is required to create a view")
			}
			if text(d, "location") != "" {
				payload["location"] = text(d, "location")
			}
		case "generic_table":
			payload["format"] = text(d, "format")
			for field, key := range map[string]string{"base_location": "base-location", "doc": "doc"} {
				if text(d, field) != "" {
					payload[key] = text(d, field)
				}
			}
		case "semantic_model":
			if err := addJSON(payload, "document", text(d, "document_json")); err != nil {
				return diag.FromErr(err)
			}
		case "policy":
			payload["type"] = text(d, "type")
			payload["description"] = text(d, "description")
			payload["content"] = text(d, "content")
		}
		if _, err := c.apiRequest(ctx, http.MethodPost, collection, payload, nil, status); err != nil {
			return diag.FromErr(err)
		}
		d.SetId(assetID(kind, d))
		if strings.HasSuffix(kind, "_registration") && len(d.Get("properties").(map[string]any)) > 0 {
			v, _, err := read(ctx, d, c)
			if err != nil {
				return diag.FromErr(err)
			}
			if _, err := c.apiRequest(ctx, http.MethodPost, assetPath(kind, d), icebergPropertyCommit(kind, d, assetObject(kind, v)), nil, 200); err != nil {
				return diag.FromErr(err)
			}
		}
		return r.ReadContext(ctx, d, m)
	}
	if kind != "generic_table" {
		r.UpdateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
			c := m.(*client)
			c.mutationMu.Lock()
			defer c.mutationMu.Unlock()
			v, _, err := read(ctx, d, c)
			if err != nil {
				return diag.FromErr(err)
			}
			obj := assetObject(kind, v)
			method := http.MethodPost
			path := assetPath(kind, d)
			payload := map[string]any{}
			switch kind {
			case "catalog":
				method = http.MethodPut
				props := objectMap(obj["properties"])
				old, new := d.GetChange("properties")
				for k := range stringMap(old) {
					delete(props, k)
				}
				for k, val := range stringMap(new) {
					props[k] = val
				}
				payload = map[string]any{"currentEntityVersion": v["entityVersion"], "properties": props}
				if err := addJSON(payload, "storageConfigInfo", text(d, "storage_config_json")); err != nil {
					return diag.FromErr(err)
				}
			case "namespace":
				path += "/properties"
				payload = propertyChanges(d)
			case "table", "table_registration", "view", "view_registration":
				payload = icebergPropertyCommit(kind, d, obj)
			case "semantic_model":
				method = http.MethodPut
				payload["entity-version"] = v["entity-version"]
				if err := addJSON(payload, "document", text(d, "document_json")); err != nil {
					return diag.FromErr(err)
				}
			case "policy":
				method = http.MethodPut
				payload = map[string]any{"current-policy-version": obj["version"], "description": text(d, "description"), "content": text(d, "content")}
			}
			if _, err := c.apiRequest(ctx, method, path, payload, nil, 200); err != nil {
				return diag.FromErr(err)
			}
			return r.ReadContext(ctx, d, m)
		}
	}
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		c := m.(*client)
		c.mutationMu.Lock()
		defer c.mutationMu.Unlock()
		path := assetPath(kind, d)
		if strings.HasPrefix(kind, "view") {
			var catalog struct {
				Properties map[string]string `json:"properties"`
			}
			status, err := c.request(ctx, http.MethodGet, "/catalogs/"+segment(text(d, "catalog")), nil, &catalog, 200, 404)
			if err != nil {
				return diag.FromErr(err)
			}
			if status == 404 {
				d.SetId("")
				return nil
			}
			if catalog.Properties["polaris.config.purge-view-metadata-on-drop"] != "false" {
				return diag.Errorf("safe view drop requires catalog property polaris.config.purge-view-metadata-on-drop=false; Polaris 1.9 otherwise purges view metadata")
			}
		}
		if strings.HasPrefix(kind, "table") {
			path += "?purgeRequested=false"
		}
		if _, err := c.apiRequest(ctx, http.MethodDelete, path, nil, nil, 204, 404); err != nil {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	r.Importer = &schema.ResourceImporter{StateContext: func(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
		if kind == "catalog" {
			return importer("name").StateContext(context.Background(), d, nil)
		}
		var id assetIdentity
		if err := json.Unmarshal([]byte(d.Id()), &id); err != nil {
			return nil, fmt.Errorf("import ID must be JSON with catalog, namespace (array), and name (except namespace resources)")
		}
		if err := id.validate(kind); err != nil {
			return nil, err
		}
		if err := d.Set("catalog", id.Catalog); err != nil {
			return nil, err
		}
		if err := d.Set("namespace", id.Namespace); err != nil {
			return nil, err
		}
		if kind != "namespace" {
			if err := d.Set("name", id.Name); err != nil {
				return nil, err
			}
		}
		return []*schema.ResourceData{d}, nil
	}}
	return r
}

type assetIdentity struct {
	Catalog   string   `json:"catalog"`
	Namespace []string `json:"namespace"`
	Name      string   `json:"name,omitempty"`
}

func (id assetIdentity) validate(kind string) error {
	names := append([]string{id.Catalog}, id.Namespace...)
	if kind != "namespace" {
		names = append(names, id.Name)
	}
	if len(id.Namespace) == 0 {
		return fmt.Errorf("namespace must contain at least one component")
	}
	for _, name := range names {
		if _, errs := validName(name, "import ID"); len(errs) > 0 || strings.ContainsRune(name, '\x1f') {
			return fmt.Errorf("invalid import identity")
		}
	}
	return nil
}
func namespaceSchema() *schema.Schema {
	return &schema.Schema{Type: schema.TypeList, Required: true, ForceNew: true, MinItems: 1, Elem: &schema.Schema{Type: schema.TypeString, ValidateFunc: func(v any, k string) ([]string, []error) {
		w, e := validName(v, k)
		if strings.ContainsRune(v.(string), '\x1f') {
			e = append(e, fmt.Errorf("namespace components cannot contain unit separators"))
		}
		return w, e
	}}, Description: "Namespace path components; dots remain literal."}
}
func namespace(d *schema.ResourceData) []string {
	out := []string{}
	for _, v := range d.Get("namespace").([]any) {
		out = append(out, v.(string))
	}
	return out
}
func assetID(kind string, d *schema.ResourceData) string {
	if kind == "catalog" {
		return text(d, "name")
	}
	id := assetIdentity{Catalog: text(d, "catalog"), Namespace: namespace(d)}
	if kind != "namespace" {
		id.Name = text(d, "name")
	}
	return encodeJSON(id)
}
func assetCollection(kind string, d *schema.ResourceData) string {
	if kind == "catalog" {
		return "/api/management/v1/catalogs"
	}
	base := "/api/catalog/v1/" + segment(text(d, "catalog")) + "/namespaces"
	if kind == "namespace" {
		return base
	}
	if kind == "generic_table" || kind == "semantic_model" || kind == "policy" {
		base = "/api/catalog/polaris/v1/" + segment(text(d, "catalog")) + "/namespaces"
	}
	base += "/" + segment(strings.Join(namespace(d), "\x1f"))
	switch kind {
	case "table":
		return base + "/tables"
	case "table_registration":
		return base + "/register"
	case "view":
		return base + "/views"
	case "view_registration":
		return base + "/register-view"
	case "generic_table":
		return base + "/generic-tables"
	case "semantic_model":
		return base + "/semantic-models"
	default:
		return base + "/policies"
	}
}
func assetPath(kind string, d *schema.ResourceData) string {
	base := assetCollection(strings.TrimSuffix(kind, "_registration"), d)
	if kind == "namespace" {
		return base + "/" + segment(strings.Join(namespace(d), "\x1f"))
	}
	return base + "/" + segment(text(d, "name"))
}
func assetObject(kind string, v map[string]any) map[string]any {
	key := ""
	switch kind {
	case "table", "table_registration", "view", "view_registration":
		key = "metadata"
	case "generic_table":
		key = "table"
	case "policy":
		key = "policy"
	}
	if key != "" {
		return objectMap(v[key])
	}
	return v
}
func validateAsset(kind string, d *schema.ResourceData, v map[string]any) error {
	obj := assetObject(kind, v)
	valid := len(obj) > 0
	switch kind {
	case "catalog":
		valid = valid && obj["name"] == text(d, "name") && obj["type"] != nil && obj["properties"] != nil
	case "namespace":
		valid = valid && encodeJSON(obj["namespace"]) == encodeJSON(namespace(d)) && obj["properties"] != nil
	case "table", "table_registration":
		valid = valid && v["metadata-location"] != nil && obj["table-uuid"] != nil
	case "view", "view_registration":
		valid = valid && v["metadata-location"] != nil && obj["view-uuid"] != nil
	case "generic_table":
		valid = valid && obj["name"] == text(d, "name") && obj["format"] != nil
	case "semantic_model":
		valid = valid && v["document"] != nil && v["entity-version"] != nil
	case "policy":
		valid = valid && obj["name"] == text(d, "name") && obj["policy-type"] != nil && obj["version"] != nil
	}
	if !valid {
		return fmt.Errorf("invalid Polaris %s response: required identity or metadata missing", kind)
	}
	return nil
}
func objectMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func stringMap(v any) map[string]string {
	out := map[string]string{}
	for k, v := range objectMap(v) {
		out[k] = v.(string)
	}
	return out
}
func encodeJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func jsonObject(v any, key string) ([]string, []error) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(v.(string)), &obj); err != nil || obj == nil {
		return nil, []error{fmt.Errorf("%s must be a JSON object", key)}
	}
	return nil, nil
}
func addJSON(payload map[string]any, key, value string) error {
	if value == "" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(value), &obj); err != nil || obj == nil {
		return fmt.Errorf("%s must be a JSON object", key)
	}
	payload[key] = obj
	return nil
}
func propertyChanges(d *schema.ResourceData) map[string]any {
	old, new := d.GetChange("properties")
	before, after := stringMap(old), stringMap(new)
	removals := []string{}
	for k := range before {
		if _, ok := after[k]; !ok {
			removals = append(removals, k)
		}
	}
	return map[string]any{"removals": removals, "updates": after}
}

func assetDataSource(kind string) *schema.Resource {
	r := assetResource(kind)
	for key, s := range r.Schema {
		identity := key == "catalog" || key == "namespace" || (key == "name" && kind != "namespace")
		s.ForceNew = false
		if !identity {
			s.Required = false
			s.Optional = false
			s.Computed = true
			s.Default = nil
			s.ValidateFunc = nil
			s.DiffSuppressFunc = nil
			s.StateFunc = nil
		}
	}
	read := r.ReadContext
	r.CreateContext = nil
	r.UpdateContext = nil
	r.DeleteContext = nil
	r.Importer = nil
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		d.SetId(assetID(kind, d))
		diags := read(ctx, d, m)
		if !diags.HasError() && d.Id() == "" {
			return diag.Errorf("Polaris %s not found", kind)
		}
		if !diags.HasError() {
			var obj map[string]any
			json.Unmarshal([]byte(text(d, "metadata_json")), &obj)
			values := map[string]any{}
			if r.Schema["properties"] != nil {
				values["properties"] = obj["properties"]
			}
			if kind == "catalog" {
				values["storage_config_json"] = encodeJSON(obj["storageConfigInfo"])
			}
			if r.Schema["location"] != nil {
				values["location"] = obj["location"]
				if kind == "namespace" {
					values["location"] = objectMap(obj["properties"])["location"]
				}
			}
			for field, selection := range map[string][3]string{"schema_json": {"schemas", "schema-id", "current-schema-id"}, "partition_spec_json": {"partition-specs", "spec-id", "default-spec-id"}, "write_order_json": {"sort-orders", "order-id", "default-sort-order-id"}, "view_version_json": {"versions", "version-id", "current-version-id"}} {
				if r.Schema[field] != nil {
					for _, entry := range interfaceList(obj[selection[0]]) {
						item := objectMap(entry)
						if item[selection[1]] == obj[selection[2]] {
							values[field] = encodeJSON(item)
						}
					}
				}
			}
			for key, value := range values {
				if value == nil {
					if key == "properties" {
						value = map[string]any{}
					} else {
						value = ""
					}
				}
				if err := d.Set(key, value); err != nil {
					return diag.FromErr(err)
				}
			}
		}
		return diags
	}
	r.Description = "Read an existing Polaris " + kind + " without granting access or persisting vended credentials."
	return r
}

func durableMetadata(kind string, obj map[string]any) map[string]any {
	if kind != "catalog" {
		return obj
	}
	out := map[string]any{}
	for k, v := range obj {
		if k != "connectionConfigInfo" {
			out[k] = v
		}
	}
	return out
}
func projectJSON(wanted, current map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range wanted {
		if remote, ok := current[k]; ok {
			if nested, ok := v.(map[string]any); ok {
				out[k] = projectJSON(nested, objectMap(remote))
			} else {
				out[k] = remote
			}
		}
	}
	return out
}

func interfaceList(v any) []any { items, _ := v.([]any); return items }
func icebergPropertyCommit(kind string, d *schema.ResourceData, obj map[string]any) map[string]any {
	changes := propertyChanges(d)
	updates := []any{}
	if len(changes["updates"].(map[string]string)) > 0 {
		updates = append(updates, map[string]any{"action": "set-properties", "updates": changes["updates"]})
	}
	if len(changes["removals"].([]string)) > 0 {
		updates = append(updates, map[string]any{"action": "remove-properties", "removals": changes["removals"]})
	}
	typ := "assert-table-uuid"
	uuid := obj["table-uuid"]
	if strings.HasPrefix(kind, "view") {
		typ = "assert-view-uuid"
		uuid = obj["view-uuid"]
	}
	return map[string]any{"requirements": []any{map[string]any{"type": typ, "uuid": uuid}}, "updates": updates}
}
