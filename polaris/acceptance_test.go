package polaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccIAMLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("run make acceptance for the owned container fixture")
	}
	u, err := url.Parse(os.Getenv("POLARIS_BASE_URL"))
	if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || os.Getenv("POLARIS_ACCEPTANCE_FIXTURE") != "owned" {
		t.Fatal("acceptance requires the owned loopback container; run make acceptance")
	}
	c, err := newClient(u.String(), os.Getenv("POLARIS_REALM"), os.Getenv("POLARIS_TOKEN"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("provider_acc_%d", time.Now().UnixNano())
	if _, err := c.request(context.Background(), http.MethodPost, "/catalogs", map[string]any{"catalog": map[string]any{"name": name, "type": "INTERNAL", "properties": map[string]string{"default-base-location": "file:///tmp/" + name}, "storageConfigInfo": map[string]string{"storageType": "FILE"}}}, nil, 201); err != nil {
		t.Fatal(err)
	}
	iceberg := func(path string, payload any) {
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, u.String()+"/api/catalog/v1/"+name+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("POLARIS_TOKEN"))
		req.Header.Set("Polaris-Realm", os.Getenv("POLARIS_REALM"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal("fixture catalog request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("fixture catalog creation failed: HTTP %d", resp.StatusCode)
		}
	}
	iceberg("/namespaces", map[string]any{"namespace": []string{"sales.q4"}})
	iceberg("/namespaces/sales.q4/tables", map[string]any{"name": "orders", "schema": map[string]any{"type": "struct", "schema-id": 0, "fields": []any{map[string]any{"id": 1, "name": "id", "required": false, "type": "long"}}}, "partition-spec": map[string]any{"spec-id": 0, "fields": []any{}}, "write-order": map[string]any{"order-id": 0, "fields": []any{}}, "stage-create": false, "properties": map[string]string{}})
	config := func(team string) string {
		return fmt.Sprintf(`
provider "polaris" {}
resource "polaris_principal" "user" {
  name = "%[1]s_user"
  properties = { team = "%[2]s" }
  credential_rotation_required = true
  lifecycle { ignore_changes = [credential_rotation_required] }
}
resource "polaris_principal_role" "group" {
  name = "%[1]s_group"
  properties = { team = "%[2]s" }
}
resource "polaris_catalog_role" "reader" {
  catalog = "%[1]s"
  name = "reader"
  properties = { team = "%[2]s" }
}
resource "polaris_principal_role_assignment" "member" {
  principal = polaris_principal.user.name
  principal_role = polaris_principal_role.group.name
}
resource "polaris_catalog_role_assignment" "binding" {
  catalog = polaris_catalog_role.reader.catalog
  catalog_role = polaris_catalog_role.reader.name
  principal_role = polaris_principal_role.group.name
}
resource "polaris_catalog_grant" "metadata" {
  catalog = polaris_catalog_role.reader.catalog
  catalog_role = polaris_catalog_role.reader.name
  privilege = "CATALOG_READ_CONFIG"
}
resource "polaris_namespace_grant" "metadata" {
  catalog = polaris_catalog_role.reader.catalog
  catalog_role = polaris_catalog_role.reader.name
  namespace = ["sales.q4"]
  privilege = "NAMESPACE_READ_PROPERTIES"
}
resource "polaris_table_grant" "read" {
  catalog = polaris_catalog_role.reader.catalog
  catalog_role = polaris_catalog_role.reader.name
  namespace = ["sales.q4"]
  table_name = "orders"
  privilege = "TABLE_READ_DATA"
}
`, name, team)
	}
	steps := []resource.TestStep{
		{Config: config("data"), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttrSet("polaris_principal.user", "client_id"), resource.TestCheckNoResourceAttr("polaris_principal.user", "client_secret"))},
		{Config: config("data"), PlanOnly: true},
		{Config: config("analytics"), Check: resource.TestCheckResourceAttr("polaris_principal.user", "properties.team", "analytics")},
	}
	imports := []struct{ address, id string }{
		{"polaris_principal.user", name + "_user"},
		{"polaris_principal_role.group", name + "_group"},
		{"polaris_catalog_role.reader", composite(name, "reader")},
		{"polaris_principal_role_assignment.member", composite(name+"_user", name+"_group")},
		{"polaris_catalog_role_assignment.binding", composite(name, name+"_group", "reader")},
	}
	for _, test := range imports {
		steps = append(steps, resource.TestStep{ResourceName: test.address, ImportState: true, ImportStateId: test.id, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"credential_rotation_required"}})
	}
	for _, g := range []struct {
		address string
		value   grant
	}{
		{"polaris_catalog_grant.metadata", grant{Type: "catalog", Privilege: "CATALOG_READ_CONFIG"}},
		{"polaris_namespace_grant.metadata", grant{Type: "namespace", Privilege: "NAMESPACE_READ_PROPERTIES", Namespace: []string{"sales.q4"}}},
		{"polaris_table_grant.read", grant{Type: "table", Privilege: "TABLE_READ_DATA", Namespace: []string{"sales.q4"}, Table: "orders"}},
	} {
		raw, _ := json.Marshal(grantID{Catalog: name, Role: "reader", Grant: g.value})
		steps = append(steps, resource.TestStep{ResourceName: g.address, ImportState: true, ImportStateId: string(raw), ImportStateVerify: true})
	}
	steps = append(steps,
		resource.TestStep{Config: config("analytics"), PlanOnly: true, ExpectNonEmptyPlan: true, PreConfig: func() {
			if _, err := c.request(context.Background(), http.MethodDelete, "/principals/"+name+"_user/principal-roles/"+name+"_group", nil, nil, 204); err != nil {
				t.Fatal(err)
			}
		}},
		resource.TestStep{Config: config("analytics")},
		resource.TestStep{Config: config("analytics"), PlanOnly: true},
	)
	resource.Test(t, resource.TestCase{ProviderFactories: map[string]func() (*schema.Provider, error){"polaris": func() (*schema.Provider, error) { return New("test")(), nil }}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
		for _, path := range []string{"/principals/" + name + "_user", "/principal-roles/" + name + "_group", "/catalogs/" + name + "/catalog-roles/reader"} {
			if _, err := c.request(context.Background(), http.MethodGet, path, nil, nil, 404); err != nil {
				return fmt.Errorf("managed fixture identity or role remained after destroy")
			}
		}
		return nil
	}})
}
