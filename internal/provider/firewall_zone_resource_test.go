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

func TestFirewallZone_basic(t *testing.T) {
	var mu sync.Mutex
	zones := map[string]map[string]any{}

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

	// Site discovery for Integration API
	mux.HandleFunc("/proxy/network/integration/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "test-site-uuid"},
			},
		})
	})

	// Firewall zone endpoints — exact path for list/create
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/zones", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Handle paginated list requests (GET with query params)
		if r.Method == http.MethodGet {
			var list []map[string]any
			for _, z := range zones {
				list = append(list, z)
			}
			if list == nil {
				list = []map[string]any{}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data":       list,
				"offset":     0,
				"limit":      200,
				"count":      len(list),
				"totalCount": len(list),
			})
			return
		}

		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)
			networkIDs := req["networkIds"]
			if networkIDs == nil {
				networkIDs = []any{}
			}
			zone := map[string]any{
				"id":         "zone-test-id-001",
				"name":       req["name"],
				"networkIds": networkIDs,
				"metadata": map[string]any{
					"origin":       "USER_DEFINED",
					"configurable": true,
				},
			}
			zones["zone-test-id-001"] = zone
			json.NewEncoder(w).Encode(zone)
			return
		}

		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Firewall zone endpoints — ID-specific paths (get, update, delete)
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/zones/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/integration/v1/sites/test-site-uuid/firewall/zones/")

		switch r.Method {
		case http.MethodGet:
			zone, ok := zones[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"message": "not found"})
				return
			}
			json.NewEncoder(w).Encode(zone)

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)
			networkIDs := req["networkIds"]
			if networkIDs == nil {
				networkIDs = []any{}
			}
			zone := map[string]any{
				"id":         id,
				"name":       req["name"],
				"networkIds": networkIDs,
				"metadata": map[string]any{
					"origin":       "USER_DEFINED",
					"configurable": true,
				},
			}
			zones[id] = zone
			json.NewEncoder(w).Encode(zone)

		case http.MethodDelete:
			delete(zones, id)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})

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
					resource "unifi_firewall_zone" "test" {
						name        = "test-zone"
						network_ids = []
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_zone.test", "name", "test-zone"),
					resource.TestCheckResourceAttr("unifi_firewall_zone.test", "origin", "USER_DEFINED"),
					resource.TestCheckResourceAttr("unifi_firewall_zone.test", "configurable", "true"),
					resource.TestCheckResourceAttrSet("unifi_firewall_zone.test", "id"),
				),
			},
		},
	})
}
