package polaris

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestAssetInventory(t *testing.T) {
	p := New("test")()
	for _, name := range []string{"catalog", "namespace", "table", "table_registration", "view", "view_registration", "generic_table", "semantic_model", "policy", "policy_attachment", "view_grant", "policy_grant", "semantic_model_grant"} {
		if p.ResourcesMap["polaris_"+name] == nil {
			t.Errorf("missing asset resource: %s", name)
		}
	}
	for _, name := range []string{"catalog", "namespace", "table", "view", "generic_table", "semantic_model", "policy"} {
		if p.DataSourcesMap["polaris_"+name] == nil {
			t.Errorf("missing read-only lookup: %s", name)
		}
	}
}

func TestAssetSafetyContracts(t *testing.T) {
	p := New("test")()
	for _, kind := range []string{"table", "table_registration", "view", "view_registration"} {
		t.Run(kind, func(t *testing.T) {
			r := p.ResourcesMap["polaris_"+kind]
			if r == nil {
				t.Fatal("resource not implemented")
			}
			var deleted bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if q.URL.Path == "/api/management/v1/catalogs/cat" {
					w.Write([]byte(`{"properties":{"polaris.config.purge-view-metadata-on-drop":"false"}}`))
					return
				}
				if !strings.HasPrefix(q.URL.Path, "/api/catalog/v1/cat/namespaces/a\x1fb/") {
					t.Errorf("incorrect namespace route: %q", q.URL.Path)
				}
				if q.Method == "DELETE" {
					deleted = true
					if strings.HasPrefix(kind, "table") && q.URL.Query().Get("purgeRequested") != "false" {
						t.Error("drop must disable purge")
					}
					w.WriteHeader(204)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"metadata-location": "file:///new.json", "metadata": map[string]any{"table-uuid": "uuid", "view-uuid": "uuid", "location": "file:///data", "properties": map[string]string{"owned": "changed", "external": "keep"}}, "config": map[string]string{"token": "never-store"}})
			}))
			defer server.Close()
			c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
			config := map[string]any{"catalog": "cat", "namespace": []any{"a", "b"}, "name": "obj", "properties": map[string]any{"owned": "initial"}}
			if strings.HasSuffix(kind, "registration") {
				config["metadata_location"] = "file:///initial.json"
			}
			d := schema.TestResourceDataRaw(t, r.Schema, config)
			d.SetId("owned")
			if diags := r.ReadContext(context.Background(), d, c); diags.HasError() {
				t.Fatal(diags)
			}
			if d.Get("current_metadata_location") != "file:///new.json" {
				t.Fatal("runtime pointer not refreshed")
			}
			if strings.HasSuffix(kind, "registration") && d.Get("metadata_location") != "file:///initial.json" {
				t.Fatal("initial pointer rewritten")
			}
			if strings.Contains(d.Get("metadata_json").(string), "never-store") {
				t.Fatal("vended token persisted")
			}
			props := d.Get("properties").(map[string]any)
			if len(props) != 1 || props["owned"] != "changed" {
				t.Fatal("external properties must remain unmanaged")
			}
			if diags := r.DeleteContext(context.Background(), d, c); diags.HasError() || !deleted || d.Id() != "" {
				t.Fatalf("delete failed: %v", diags)
			}
		})
	}
}

func TestViewDropRefusesPurgeDefault(t *testing.T) {
	for _, kind := range []string{"view", "view_registration"} {
		t.Run(kind, func(t *testing.T) {
			deleted := false
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if q.Method == "DELETE" {
					deleted = true
					w.WriteHeader(204)
					return
				}
				w.Write([]byte(`{"properties":{}}`))
			}))
			defer s.Close()
			c, _ := newClient(s.URL, "fixture", "token", "", "")
			r := assetResource(kind)
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"catalog": "cat", "namespace": []any{"a"}, "name": "obj"})
			d.SetId("keep")
			if !r.DeleteContext(context.Background(), d, c).HasError() || deleted || d.Id() != "keep" {
				t.Fatal("view drop must refuse server default metadata purge and preserve state")
			}
		})
	}
}

func TestAssetMalformedReadPreservesState(t *testing.T) {
	p := New("test")()
	for _, kind := range []string{"catalog", "namespace", "table", "view", "generic_table", "semantic_model", "policy"} {
		t.Run(kind, func(t *testing.T) {
			r := p.ResourcesMap["polaris_"+kind]
			if r == nil {
				t.Fatal("resource not implemented")
			}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{}`)) }))
			defer s.Close()
			c, _ := newClient(s.URL, "fixture", "token", "", "")
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"catalog": "cat", "namespace": []any{"a"}, "name": "obj"})
			d.SetId("keep")
			if !r.ReadContext(context.Background(), d, c).HasError() || d.Id() != "keep" {
				t.Fatal("malformed success must preserve state and fail")
			}
		})
	}
}

func TestAssetDataSourceReturnsDurableFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name": "cat", "type": "INTERNAL",
			"properties":        map[string]string{"owner": "team"},
			"storageConfigInfo": map[string]any{"storageType": "FILE", "allowedLocations": []string{"file:///data"}},
		})
	}))
	defer s.Close()
	c, _ := newClient(s.URL, "fixture", "token", "", "")
	r := assetDataSource("catalog")
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"name": "cat"})
	if diags := r.ReadContext(context.Background(), d, c); diags.HasError() {
		t.Fatal(diags)
	}
	if got := d.Get("properties").(map[string]any)["owner"]; got != "team" {
		t.Errorf("lookup omitted existing property: owner=%v", got)
	}
	var storage map[string]any
	if err := json.Unmarshal([]byte(text(d, "storage_config_json")), &storage); err != nil || storage["storageType"] != "FILE" {
		t.Errorf("lookup omitted storage configuration: %q", text(d, "storage_config_json"))
	}
}

func TestAssetRegistrationAppliesDeclaredProperties(t *testing.T) {
	for _, kind := range []string{"table_registration", "view_registration"} {
		t.Run(kind, func(t *testing.T) {
			properties := map[string]string{"owned": "before", "external": "keep"}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if q.Method == http.MethodPost {
					var payload struct {
						Updates []struct {
							Action  string            `json:"action"`
							Updates map[string]string `json:"updates"`
						} `json:"updates"`
					}
					if err := json.NewDecoder(q.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					for _, update := range payload.Updates {
						if update.Action == "set-properties" {
							for key, value := range update.Updates {
								properties[key] = value
							}
						}
					}
				}
				json.NewEncoder(w).Encode(map[string]any{
					"metadata-location": "file:///current.json",
					"metadata":          map[string]any{"table-uuid": "table-id", "view-uuid": "view-id", "properties": properties},
				})
			}))
			defer s.Close()
			c, _ := newClient(s.URL, "fixture", "token", "", "")
			r := assetResource(kind)
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
				"catalog": "cat", "namespace": []any{"ns"}, "name": "obj",
				"metadata_location": "file:///initial.json", "properties": map[string]any{"owned": "desired"},
			})
			if diags := r.CreateContext(context.Background(), d, c); diags.HasError() {
				t.Fatal(diags)
			}
			if properties["owned"] != "desired" || properties["external"] != "keep" {
				t.Errorf("registration did not reconcile owned properties while preserving external ones: %v", properties)
			}
			if got := d.Get("properties").(map[string]any)["owned"]; got != "desired" {
				t.Errorf("registered property state differs from configuration: owned=%v", got)
			}
		})
	}
}
