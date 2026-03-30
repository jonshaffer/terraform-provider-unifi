package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestFirewallGroup_basic(t *testing.T) {
	var mu sync.Mutex
	groups := map[string]map[string]any{}

	mux := http.NewServeMux()

	// Feature detection: OpenAPI spec
	mux.HandleFunc("/proxy/network/api-docs/integration.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"info":  map[string]any{"version": "1.0.0"},
			"paths": map[string]any{},
		})
	})

	// Feature detection: feature flags
	mux.HandleFunc("/proxy/network/v2/api/site/default/features", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{})
	})

	// Firewall group endpoints — exact path for list/create
	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallgroup", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			var list []map[string]any
			for _, g := range groups {
				list = append(list, g)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": list,
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var group map[string]any
			json.Unmarshal(body, &group)
			group["_id"] = "fwg-test-id-001"
			groups["fwg-test-id-001"] = group
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{group},
				"meta": map[string]any{"rc": "ok"},
			})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Firewall group endpoints — ID-specific paths (get, update, delete)
	mux.HandleFunc("/proxy/network/api/s/default/rest/firewallgroup/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/api/s/default/rest/firewallgroup/")

		switch r.Method {
		case http.MethodGet:
			group, ok := groups[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{
					"data": []any{},
					"meta": map[string]any{"rc": "error", "msg": "not found"},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{group},
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var group map[string]any
			json.Unmarshal(body, &group)
			group["_id"] = id
			groups[id] = group
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{group},
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodDelete:
			delete(groups, id)
			json.NewEncoder(w).Encode(map[string]any{
				"data": []any{},
				"meta": map[string]any{"rc": "ok"},
			})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("UNIFI_API_KEY", "test-key")
	t.Setenv("UNIFI_BASE_URL", srv.URL)
	t.Setenv("UNIFI_SITE", "default")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "unifi_firewall_group" "test" {
						name    = "test-ip-group"
						type    = "address-group"
						members = ["192.168.1.0/24", "10.0.0.0/8"]
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_group.test", "name", "test-ip-group"),
					resource.TestCheckResourceAttr("unifi_firewall_group.test", "type", "address-group"),
					resource.TestCheckResourceAttr("unifi_firewall_group.test", "members.#", "2"),
					resource.TestCheckResourceAttr("unifi_firewall_group.test", "members.0", "192.168.1.0/24"),
					resource.TestCheckResourceAttr("unifi_firewall_group.test", "members.1", "10.0.0.0/8"),
					resource.TestCheckResourceAttrSet("unifi_firewall_group.test", "id"),
				),
			},
		},
	})
}
