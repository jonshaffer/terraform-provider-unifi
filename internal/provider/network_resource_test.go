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

func TestNetwork_basic(t *testing.T) {
	var mu sync.Mutex
	networks := map[string]map[string]any{}

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

	// Network endpoints — exact path for list/create
	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			var list []map[string]any
			for _, n := range networks {
				list = append(list, n)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": list,
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var net map[string]any
			json.Unmarshal(body, &net)
			net["_id"] = "net-test-id-001"
			networks["net-test-id-001"] = net
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{net},
				"meta": map[string]any{"rc": "ok"},
			})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Network endpoints — ID-specific paths (get, update, delete)
	mux.HandleFunc("/proxy/network/api/s/default/rest/networkconf/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/api/s/default/rest/networkconf/")

		switch r.Method {
		case http.MethodGet:
			net, ok := networks[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{
					"data": []any{},
					"meta": map[string]any{"rc": "error", "msg": "not found"},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{net},
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var net map[string]any
			json.Unmarshal(body, &net)
			net["_id"] = id
			networks[id] = net
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{net},
				"meta": map[string]any{"rc": "ok"},
			})

		case http.MethodDelete:
			delete(networks, id)
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
					resource "unifi_network" "test" {
						name         = "TestVLAN"
						purpose      = "corporate"
						vlan         = 100
						vlan_enabled = true
						ip_subnet    = "10.0.100.0/24"
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_network.test", "name", "TestVLAN"),
					resource.TestCheckResourceAttr("unifi_network.test", "purpose", "corporate"),
					resource.TestCheckResourceAttr("unifi_network.test", "vlan", "100"),
					resource.TestCheckResourceAttr("unifi_network.test", "vlan_enabled", "true"),
					resource.TestCheckResourceAttr("unifi_network.test", "ip_subnet", "10.0.100.0/24"),
					resource.TestCheckResourceAttr("unifi_network.test", "dhcpd_enabled", "false"),
					resource.TestCheckResourceAttr("unifi_network.test", "internet_access_enabled", "true"),
					resource.TestCheckResourceAttrSet("unifi_network.test", "id"),
				),
			},
		},
	})
}
