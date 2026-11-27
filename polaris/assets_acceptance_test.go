package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func acceptanceClient(t *testing.T) *client {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("run an owned acceptance fixture")
	}
	u, err := url.Parse(os.Getenv("POLARIS_BASE_URL"))
	if err != nil || u.Hostname() != "127.0.0.1" || os.Getenv("POLARIS_ACCEPTANCE_FIXTURE") != "owned" {
		t.Fatal("owned loopback fixture required")
	}
	c, err := newClient(u.String(), "POLARIS", os.Getenv("POLARIS_TOKEN"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = fixtureDiagnostics{t: t}
	return c
}

type fixtureDiagnostics struct{ t *testing.T }

func (f fixtureDiagnostics) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		var payload struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(raw, &payload)
		message := strings.ReplaceAll(payload.Error.Message, os.Getenv("POLARIS_TOKEN"), "<redacted>")
		f.t.Logf("fixture %s %s: HTTP %d %s %s", req.Method, req.URL.Path, resp.StatusCode, payload.Error.Type, message)
	}
	return resp, err
}

func TestAccAssetsLifecycle(t *testing.T) {
	c := acceptanceClient(t)
	name := fmt.Sprintf("assets_%d", time.Now().UnixNano())
	ctx := context.Background()
	config := func(team string) string {
		return fmt.Sprintf(`
provider "polaris" {}
resource "polaris_catalog" "this" {
 name = "%[1]s"
 properties = { "default-base-location" = "file:///tmp/%[1]s", team = "%[2]s", "polaris.config.purge-view-metadata-on-drop" = "false" }
 storage_config_json = jsonencode({ storageType = "FILE", allowedLocations = ["file:///tmp/%[1]s"] })
}
resource "polaris_namespace" "this" {
 catalog = polaris_catalog.this.name
 namespace = ["sales.q4"]
 properties = { team = "%[2]s" }
}
resource "polaris_table" "this" {
 catalog = polaris_catalog.this.name
 namespace = polaris_namespace.this.namespace
 name = "orders"
 schema_json = jsonencode({type="struct", "schema-id"=0, fields=[{id=1,name="id",required=false,type="long"}]})
 properties = { team = "%[2]s" }
}
resource "polaris_view" "this" {
 catalog = polaris_catalog.this.name
 namespace = polaris_namespace.this.namespace
 name = "orders_view"
 schema_json = jsonencode({type="struct", "schema-id"=0, fields=[{id=1,name="id",required=false,type="long"}]})
 view_version_json = jsonencode({"version-id"=1,"timestamp-ms"=1700000000000,"schema-id"=0,summary={},representations=[{type="sql",sql="select 1 as id",dialect="spark"}],"default-namespace"=["sales.q4"]})
 properties = { team = "%[2]s" }
}
resource "polaris_generic_table" "this" {
 catalog = polaris_catalog.this.name
 namespace = polaris_namespace.this.namespace
 name = "raw"
 format = "csv"
 base_location = "file:///tmp/%[1]s/sales.q4/raw"
 doc = "Owned fixture"
 properties = { team = "data" }
}
resource "polaris_semantic_model" "this" {
 catalog = polaris_catalog.this.name
 namespace = polaris_namespace.this.namespace
 name = "metrics"
 document_json = jsonencode({version="0.2.0.dev0",semantic_model=jsonencode({name="%[2]s",datasets=[]})})
}
resource "polaris_policy" "this" {
 catalog = polaris_catalog.this.name
 namespace = polaris_namespace.this.namespace
 name = "compaction"
 type = "system.data-compaction"
 description = "%[2]s"
 content = jsonencode({version="2025-02-03",enable=true,config={target_file_size_bytes=134217728}})
}
resource "polaris_policy_attachment" "this" {
 catalog = polaris_policy.this.catalog
 namespace = polaris_policy.this.namespace
 name = polaris_policy.this.name
 target_type = "table-like"
 target_path = concat(polaris_table.this.namespace,[polaris_table.this.name])
}
resource "polaris_catalog_role" "this" {
 catalog = polaris_catalog.this.name
 name = "asset_reader"
}
resource "polaris_view_grant" "this" {
 catalog = polaris_catalog_role.this.catalog
 catalog_role = polaris_catalog_role.this.name
 namespace = polaris_view.this.namespace
 view_name = polaris_view.this.name
 privilege = "VIEW_READ_PROPERTIES"
}
resource "polaris_policy_grant" "this" {
 catalog = polaris_catalog_role.this.catalog
 catalog_role = polaris_catalog_role.this.name
 namespace = polaris_policy.this.namespace
 policy_name = polaris_policy.this.name
 privilege = "POLICY_READ"
}
resource "polaris_semantic_model_grant" "this" {
 catalog = polaris_catalog_role.this.catalog
 catalog_role = polaris_catalog_role.this.name
 namespace = polaris_semantic_model.this.namespace
 semantic_model_name = polaris_semantic_model.this.name
 privilege = "SEMANTIC_MODEL_READ"
}
`, name, team) + assetLookupConfig()
	}
	steps := []resource.TestStep{{Config: config("data")}, {Config: config("data"), PlanOnly: true}, {Config: config("analytics"), PreConfig: func() {
		// Add engine-owned keys and advance table/view metadata outside Terraform.
		for _, kind := range []string{"catalog", "namespace", "table", "view"} {
			r := New("test")().ResourcesMap["polaris_"+kind]
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"catalog": name, "namespace": []any{"sales.q4"}, "name": map[string]string{"catalog": name, "table": "orders", "view": "orders_view"}[kind]})
			path := assetPath(kind, d)
			method := "POST"
			payload := map[string]any{}
			if kind == "catalog" {
				var v map[string]any
				if _, err := c.apiRequest(ctx, "GET", path, nil, &v, 200); err != nil {
					t.Fatal(err)
				}
				props := objectMap(v["properties"])
				props["external"] = "keep"
				payload = map[string]any{"currentEntityVersion": v["entityVersion"], "properties": props}
				method = "PUT"
			} else if kind == "namespace" {
				path += "/properties"
				payload = map[string]any{"updates": map[string]string{"external": "keep"}}
			} else {
				payload = map[string]any{"updates": []any{map[string]any{"action": "set-properties", "updates": map[string]string{"external": "keep"}}}}
			}
			if _, err := c.apiRequest(ctx, method, path, payload, nil, 200); err != nil {
				t.Fatal(err)
			}
		}
	}, Check: func(_ *terraform.State) error {
		for _, kind := range []string{"catalog", "namespace", "table", "view"} {
			r := New("test")().ResourcesMap["polaris_"+kind]
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"catalog": name, "namespace": []any{"sales.q4"}, "name": map[string]string{"catalog": name, "table": "orders", "view": "orders_view"}[kind]})
			var v map[string]any
			if _, err := c.apiRequest(ctx, "GET", assetPath(kind, d), nil, &v, 200); err != nil {
				return err
			}
			props := objectMap(assetObject(kind, v)["properties"])
			if props["external"] != "keep" || props["team"] != "analytics" {
				return fmt.Errorf("%s update lost external properties or failed reconciliation", kind)
			}
		}
		for _, kind := range []string{"catalog", "namespace"} {
			r := New("test")().ResourcesMap["polaris_"+kind]
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"name": name, "catalog": name, "namespace": []any{"sales.q4"}})
			if _, err := c.apiRequest(ctx, "DELETE", assetPath(kind, d), nil, nil, 400, 409); err != nil {
				return fmt.Errorf("nonempty %s deletion did not refuse", kind)
			}
		}
		return nil
	}}, {Config: config("analytics"), PlanOnly: true}}
	// Import adopts identity and current metadata; creation-only values cannot be reconstructed.
	for _, kind := range []string{"catalog", "namespace", "table", "view", "generic_table", "semantic_model", "policy"} {
		id := name
		if kind != "catalog" {
			object := map[string]string{"table": "orders", "view": "orders_view", "generic_table": "raw", "semantic_model": "metrics", "policy": "compaction"}[kind]
			id = encodeJSON(assetIdentity{Catalog: name, Namespace: []string{"sales.q4"}, Name: object})
		}
		steps = append(steps, resource.TestStep{ResourceName: "polaris_" + kind + ".this", ImportState: true, ImportStateId: id, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"properties", "storage_config_json", "schema_json", "view_version_json", "partition_spec_json", "write_order_json", "location"}})
	}
	steps = append(steps, resource.TestStep{ResourceName: "polaris_policy_attachment.this", ImportState: true, ImportStateId: encodeJSON(attachmentID{Catalog: name, Namespace: []string{"sales.q4"}, Name: "compaction", TargetType: "table-like", TargetPath: []string{"sales.q4", "orders"}}), ImportStateVerify: true})
	for _, kind := range []string{"view", "policy", "semantic_model"} {
		g := grant{Type: kind, Namespace: []string{"sales.q4"}}
		switch kind {
		case "view":
			g.View = "orders_view"
			g.Privilege = "VIEW_READ_PROPERTIES"
		case "policy":
			g.Policy = "compaction"
			g.Privilege = "POLICY_READ"
		default:
			g.Type = "semantic-model"
			g.Model = "metrics"
			g.Privilege = "SEMANTIC_MODEL_READ"
		}
		steps = append(steps, resource.TestStep{ResourceName: "polaris_" + kind + "_grant.this", ImportState: true, ImportStateId: encodeJSON(grantID{Catalog: name, Role: "asset_reader", Grant: g}), ImportStateVerify: true})
	}
	resource.Test(t, resource.TestCase{ProviderFactories: map[string]func() (*schema.Provider, error){"polaris": func() (*schema.Provider, error) { return New("test")(), nil }}, Steps: steps})
}
func assetLookupConfig() string {
	out := ""
	for _, kind := range []string{"catalog", "namespace", "table", "view", "generic_table", "semantic_model", "policy"} {
		out += "\ndata \"polaris_" + kind + "\" \"this\" {\n"
		if kind == "catalog" {
			out += "name = polaris_catalog.this.name\n"
		} else {
			out += "catalog = polaris_" + kind + ".this.catalog\nnamespace = polaris_" + kind + ".this.namespace\n"
			if kind != "namespace" {
				out += "name = polaris_" + kind + ".this.name\n"
			}
		}
		out += "}\n"
	}
	return out
}

func TestAccRegistrationPreservesMetadata(t *testing.T) {
	c := acceptanceClient(t)
	ctx := context.Background()
	name := fmt.Sprintf("registration_%d", time.Now().UnixNano())
	p := New("test")()
	cat := p.ResourcesMap["polaris_catalog"]
	cd := schema.TestResourceDataRaw(t, cat.Schema, map[string]any{"name": name, "properties": map[string]any{"default-base-location": "file:///tmp/" + name, "polaris.config.purge-view-metadata-on-drop": "false"}, "storage_config_json": `{"storageType":"FILE"}`})
	if ds := cat.CreateContext(ctx, cd, c); ds.HasError() {
		t.Fatal(ds)
	}
	defer func() {
		if ds := cat.DeleteContext(ctx, cd, c); ds.HasError() {
			t.Error(ds)
		}
	}()
	ns := p.ResourcesMap["polaris_namespace"]
	nd := schema.TestResourceDataRaw(t, ns.Schema, map[string]any{"catalog": name, "namespace": []any{"a.b"}})
	if ds := ns.CreateContext(ctx, nd, c); ds.HasError() {
		t.Fatal(ds)
	}
	defer func() {
		if ds := ns.DeleteContext(ctx, nd, c); ds.HasError() {
			t.Error(ds)
		}
	}()
	for _, kind := range []string{"table", "view"} {
		t.Run(kind, func(t *testing.T) {
			r := p.ResourcesMap["polaris_"+kind]
			config := map[string]any{"catalog": name, "namespace": []any{"a.b"}, "name": "object", "schema_json": `{"type":"struct","schema-id":0,"fields":[{"id":1,"name":"id","required":false,"type":"long"}]}`}
			if kind == "view" {
				config["view_version_json"] = `{"version-id":1,"timestamp-ms":1700000000000,"schema-id":0,"summary":{},"representations":[{"type":"sql","sql":"select 1 as id","dialect":"spark"}],"default-namespace":["a.b"]}`
			}
			d := schema.TestResourceDataRaw(t, r.Schema, config)
			if ds := r.CreateContext(ctx, d, c); ds.HasError() {
				t.Fatal(ds)
			}
			initial := text(d, "current_metadata_location")
			if ds := r.DeleteContext(ctx, d, c); ds.HasError() {
				t.Fatal(ds)
			}
			reg := p.ResourcesMap["polaris_"+kind+"_registration"]
			config["metadata_location"] = initial
			delete(config, "schema_json")
			delete(config, "view_version_json")
			rd := schema.TestResourceDataRaw(t, reg.Schema, config)
			if ds := reg.CreateContext(ctx, rd, c); ds.HasError() {
				t.Fatal(ds)
			}
			defer func() {
				if ds := reg.DeleteContext(ctx, rd, c); ds.HasError() {
					t.Error(ds)
				}
			}()
			if ds := reg.CreateContext(ctx, schema.TestResourceDataRaw(t, reg.Schema, config), c); !ds.HasError() {
				t.Fatal("registration must not overwrite an existing name")
			}
			before := text(rd, "current_metadata_location")
			if _, err := c.apiRequest(ctx, http.MethodPost, assetPath(kind, rd), map[string]any{"updates": []any{map[string]any{"action": "set-properties", "updates": map[string]string{"external": "commit"}}}}, nil, 200); err != nil {
				t.Fatal(err)
			}
			if ds := reg.ReadContext(ctx, rd, c); ds.HasError() {
				t.Fatal(ds)
			}
			if text(rd, "metadata_location") != initial || text(rd, "current_metadata_location") == before {
				t.Fatal("external commit reset initial metadata or failed to advance pointer")
			}
			imported := schema.TestResourceDataRaw(t, reg.Schema, nil)
			imported.SetId(rd.Id())
			if _, err := reg.Importer.StateContext(ctx, imported, c); err != nil {
				t.Fatal(err)
			}
			if ds := reg.ReadContext(ctx, imported, c); ds.HasError() {
				t.Fatal(ds)
			}
			if ds := reg.DeleteContext(ctx, rd, c); ds.HasError() {
				t.Fatal(ds)
			}
			// Successful re-registration of the same original metadata proves unregister did not delete it.
			again := schema.TestResourceDataRaw(t, reg.Schema, config)
			if ds := reg.CreateContext(ctx, again, c); ds.HasError() {
				t.Fatal(ds)
			}
			if ds := reg.DeleteContext(ctx, again, c); ds.HasError() {
				t.Fatal(ds)
			}
			if !strings.HasPrefix(initial, "file:") {
				t.Fatal("fixture escaped FILE storage")
			}
		})
	}
}
