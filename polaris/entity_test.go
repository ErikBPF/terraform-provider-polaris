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

func TestEntityLifecycle(t *testing.T) {
	for _, kind := range []string{"principal", "principalRole", "catalogRole"} {
		t.Run(kind, func(t *testing.T) {
			var stored entity
			var created, updated, deleted bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					var payload map[string]json.RawMessage
					json.NewDecoder(r.Body).Decode(&payload)
					if err := json.Unmarshal(payload[kind], &stored); err != nil {
						t.Error(err)
					}
					if kind == "principal" && string(payload["credentialRotationRequired"]) != "true" {
						t.Error("rotation marker missing")
					}
					stored.Version = 7
					stored.ClientID = "fixture-client"
					created = true
					w.WriteHeader(201)
					if kind == "principal" {
						json.NewEncoder(w).Encode(map[string]any{"principal": stored, "credentials": map[string]string{"clientId": "fixture-client", "clientSecret": "must-not-enter-state"}})
					} else {
						json.NewEncoder(w).Encode(stored)
					}
				case http.MethodPut:
					var payload struct {
						Version    int64             `json:"currentEntityVersion"`
						Properties map[string]string `json:"properties"`
					}
					json.NewDecoder(r.Body).Decode(&payload)
					if payload.Version != 7 {
						t.Error("update did not use latest server version")
					}
					stored.Properties = payload.Properties
					stored.Version++
					updated = true
					json.NewEncoder(w).Encode(stored)
				case http.MethodDelete:
					deleted = true
					w.WriteHeader(204)
				default:
					json.NewEncoder(w).Encode(stored)
				}
			}))
			defer server.Close()
			c, err := newClient(server.URL, "fixture", "fixture-token", "", "")
			if err != nil {
				t.Fatal(err)
			}
			r := entityResource(kind)
			config := map[string]any{"name": "alice", "properties": map[string]any{"team": "data"}}
			if kind == "catalogRole" {
				config["catalog"] = "warehouse"
			}
			if kind == "principal" {
				config["credential_rotation_required"] = true
			}
			d := schema.TestResourceDataRaw(t, r.Schema, config)
			if diagnostics := r.CreateContext(context.Background(), d, c); diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			if !created || d.Id() == "" || d.Get("name") != "alice" {
				t.Fatal("create did not persist identity")
			}
			if _, exists := r.Schema["client_secret"]; exists {
				t.Fatal("principal must not persist generated credentials")
			}
			d.Set("properties", map[string]any{"team": "analytics"})
			if diagnostics := r.UpdateContext(context.Background(), d, c); diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			if !updated || !reflect.DeepEqual(stored.Properties, map[string]string{"team": "analytics"}) {
				t.Fatal("properties not updated")
			}
			if diagnostics := r.DeleteContext(context.Background(), d, c); diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			if !deleted || d.Id() != "" {
				t.Fatal("delete did not clear state")
			}
		})
	}
}

func TestReadNeverTreatsForbiddenAsMissing(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
			r := resourcePrincipal()
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"name": "alice"})
			d.SetId("alice")
			diagnostics := r.ReadContext(context.Background(), d, c)
			if status == 403 && (!diagnostics.HasError() || d.Id() != "alice") {
				t.Fatal("forbidden must preserve state and fail")
			}
			if status == 404 && (diagnostics.HasError() || d.Id() != "") {
				t.Fatal("missing object must clear state")
			}
		})
	}
}

func TestImportIdentity(t *testing.T) {
	for _, test := range []struct {
		r         *schema.Resource
		id        string
		wantError bool
	}{
		{resourcePrincipal(), "alice", false},
		{resourceCatalogRole(), `["warehouse","analyst"]`, false},
		{resourceCatalogRole(), `warehouse/analyst`, true},
		{resourceCatalogRole(), `["warehouse",""]`, true},
	} {
		d := schema.TestResourceDataRaw(t, test.r.Schema, nil)
		d.SetId(test.id)
		_, err := test.r.Importer.StateContext(context.Background(), d, nil)
		if (err != nil) != test.wantError {
			t.Errorf("import error %v, expected error %v", err, test.wantError)
		}
	}
}
