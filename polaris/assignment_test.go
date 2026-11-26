package polaris

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestAssignmentLifecycle(t *testing.T) {
	for _, catalog := range []bool{false, true} {
		label := "principal"
		if catalog {
			label = "catalog"
		}
		t.Run(label, func(t *testing.T) {
			present := false
			var lastPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				lastPath = r.URL.EscapedPath()
				switch r.Method {
				case http.MethodPut:
					present = true
					w.WriteHeader(201)
				case http.MethodDelete:
					present = false
					w.WriteHeader(204)
				default:
					roles := []entity{}
					if present {
						roles = append(roles, entity{Name: "group/analysts"})
					}
					json.NewEncoder(w).Encode(map[string]any{"roles": roles})
				}
			}))
			defer server.Close()
			c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
			r := assignmentResource(catalog)
			config := map[string]any{"principal": "alice/etl", "principal_role": "group/analysts"}
			if catalog {
				config = map[string]any{"catalog": "warehouse", "principal_role": "team", "catalog_role": "group/analysts"}
			}
			d := schema.TestResourceDataRaw(t, r.Schema, config)
			if diags := r.CreateContext(context.Background(), d, c); diags.HasError() || d.Id() == "" {
				t.Fatalf("assignment create failed: %v", diags)
			}
			imported := schema.TestResourceDataRaw(t, r.Schema, nil)
			imported.SetId(d.Id())
			if _, err := r.Importer.StateContext(context.Background(), imported, nil); err != nil {
				t.Fatal(err)
			}
			if diags := r.DeleteContext(context.Background(), d, c); diags.HasError() || d.Id() != "" {
				t.Fatalf("assignment delete failed: %v", diags)
			}
			if lastPath[len(lastPath)-len("group%2Fanalysts"):] != "group%2Fanalysts" {
				t.Fatal("role path component was not escaped")
			}
			present = false
			d.SetId("old")
			if diags := r.ReadContext(context.Background(), d, c); diags.HasError() || d.Id() != "" {
				t.Fatal("removed membership must be detected")
			}
		})
	}
}
