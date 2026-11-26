package polaris

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestGrantLifecycle(t *testing.T) {
	for _, kind := range []string{"catalog", "namespace", "table", "view", "policy", "semantic_model"} {
		t.Run(kind, func(t *testing.T) {
			var stored []grant
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					json.NewEncoder(w).Encode(map[string]any{"grants": stored})
					return
				}
				var payload struct {
					Grant grant `json:"grant"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if r.Method == http.MethodPut {
					stored = append(stored, payload.Grant)
				} else {
					remaining := []grant{}
					for _, g := range stored {
						if !reflect.DeepEqual(g, payload.Grant) {
							remaining = append(remaining, g)
						}
					}
					stored = remaining
				}
				w.WriteHeader(201)
			}))
			defer server.Close()
			c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
			r := grantResource(kind)
			config := map[string]any{"catalog": "warehouse", "catalog_role": "analysts", "privilege": map[string]string{"catalog": "TABLE_READ_DATA", "namespace": "TABLE_READ_DATA", "table": "TABLE_READ_DATA", "view": "VIEW_READ_PROPERTIES", "policy": "POLICY_READ", "semantic_model": "SEMANTIC_MODEL_READ"}[kind]}
			if kind != "catalog" {
				config["namespace"] = []any{"sales.q4", "region"}
			}
			if field := map[string]string{"table": "table_name", "view": "view_name", "policy": "policy_name", "semantic_model": "semantic_model_name"}[kind]; field != "" {
				config[field] = "orders"
			}
			d := schema.TestResourceDataRaw(t, r.Schema, config)
			if diags := r.CreateContext(context.Background(), d, c); diags.HasError() || d.Id() == "" {
				t.Fatalf("grant create failed: %v", diags)
			}
			if kind != "catalog" && !reflect.DeepEqual(stored[0].Namespace, []string{"sales.q4", "region"}) {
				t.Fatal("namespace components lost")
			}
			imported := schema.TestResourceDataRaw(t, r.Schema, nil)
			imported.SetId(d.Id())
			if _, err := r.Importer.StateContext(context.Background(), imported, nil); err != nil {
				t.Fatal(err)
			}
			other := stored[0]
			other.Privilege = "TABLE_WRITE_DATA"
			stored = append(stored, other)
			if diags := r.DeleteContext(context.Background(), d, c); diags.HasError() || d.Id() != "" {
				t.Fatalf("grant delete failed: %v", diags)
			}
			if len(stored) != 1 || stored[0].Privilege != "TABLE_WRITE_DATA" {
				t.Fatal("delete changed another grant")
			}
			d.SetId("old")
			if diags := r.ReadContext(context.Background(), d, c); diags.HasError() || d.Id() != "" {
				t.Fatal("missing grant must clear state")
			}
			if diags := r.DeleteContext(context.Background(), d, c); diags.HasError() {
				t.Fatalf("missing grant delete must be idempotent: %v", diags)
			}
		})
	}
}

func TestGrantReadRequiresExactNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"grants":[{"type":"table","privilege":"TABLE_READ_DATA","namespace":["sales","q4"],"tableName":"orders"}]}`))
	}))
	defer server.Close()
	c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
	r := resourceTableGrant()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"catalog": "warehouse", "catalog_role": "analysts", "privilege": "TABLE_READ_DATA", "namespace": []any{"sales.q4"}, "table_name": "orders"})
	d.SetId("old")
	if diags := r.ReadContext(context.Background(), d, c); diags.HasError() || d.Id() != "" {
		t.Fatal("sibling namespace must not satisfy managed grant")
	}
}
