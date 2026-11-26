package polaris

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderSchema(t *testing.T) {
	p := New("test")()
	if err := p.InternalValidate(); err != nil {
		t.Fatal(err)
	}
	if len(p.ResourcesMap) != 8 {
		t.Fatalf("expected eight IAM resources, got %d", len(p.ResourcesMap))
	}
	for name, r := range p.ResourcesMap {
		if r.Importer == nil {
			t.Errorf("%s lacks import support", name)
		}
	}
	for _, field := range []string{"token", "client_secret"} {
		if !p.Schema[field].Sensitive {
			t.Errorf("%s must be sensitive", field)
		}
	}
}

func TestNamesRejectReservedAndTraversalSegments(t *testing.T) {
	for _, name := range []string{".", "..", "system$internal", " SYSTEM$internal", ""} {
		_, errs := nameSchema("test").ValidateFunc(name, "name")
		if len(errs) == 0 {
			t.Errorf("unsafe or reserved name was accepted: %q", name)
		}
	}
}

func TestMalformedReadPreservesState(t *testing.T) {
	for _, test := range []struct {
		name   string
		r      *schema.Resource
		config map[string]any
	}{
		{"entity", resourcePrincipal(), map[string]any{"name": "alice"}},
		{"assignment", resourcePrincipalRoleAssignment(), map[string]any{"principal": "alice", "principal_role": "analysts"}},
		{"grant", resourceCatalogGrant(), map[string]any{"catalog": "warehouse", "catalog_role": "analysts", "privilege": "TABLE_READ_DATA"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, body := range []string{`{}`, `null`, `{"roles":null,"grants":null}`} {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(body)) }))
				c, _ := newClient(server.URL, "fixture", "fixture-token", "", "")
				d := schema.TestResourceDataRaw(t, test.r.Schema, test.config)
				d.SetId("preserved")
				diagnostics := test.r.ReadContext(context.Background(), d, c)
				server.Close()
				if !diagnostics.HasError() || d.Id() != "preserved" {
					t.Errorf("malformed success must fail and preserve state")
				}
			}
		})
	}
}
