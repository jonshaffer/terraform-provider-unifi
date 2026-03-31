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

func TestFirewallPolicy_withPortFilter(t *testing.T) {
	var mu sync.Mutex
	policies := map[string]map[string]any{}

	mux := http.NewServeMux()

	mux.HandleFunc("/proxy/network/api-docs/integration.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"info": map[string]any{"version": "1.0.0"}, "paths": map[string]any{}})
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/features", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{})
	})
	mux.HandleFunc("/proxy/network/integration/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "test-site-uuid"}}})
	})

	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodGet {
			var list []map[string]any
			for _, p := range policies {
				list = append(list, p)
			}
			if list == nil {
				list = []map[string]any{}
			}
			json.NewEncoder(w).Encode(map[string]any{"data": list, "offset": 0, "limit": 200, "count": len(list), "totalCount": len(list)})
			return
		}

		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)
			policy := map[string]any{
				"id":              "policy-filter-001",
				"name":            req["name"],
				"enabled":         req["enabled"],
				"index":           0,
				"action":          req["action"],
				"source":          req["source"],
				"destination":     req["destination"],
				"ipProtocolScope": req["ipProtocolScope"],
				"loggingEnabled":  req["loggingEnabled"],
				"metadata":        map[string]any{"origin": "USER_DEFINED"},
			}
			policies["policy-filter-001"] = policy
			json.NewEncoder(w).Encode(policy)
			return
		}
	})

	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/")

		switch r.Method {
		case http.MethodGet:
			policy, ok := policies[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"message": "not found"})
				return
			}
			json.NewEncoder(w).Encode(policy)
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)
			policy := map[string]any{
				"id": id, "name": req["name"], "enabled": req["enabled"], "index": 0,
				"action": req["action"], "source": req["source"], "destination": req["destination"],
				"ipProtocolScope": req["ipProtocolScope"], "loggingEnabled": req["loggingEnabled"],
				"metadata": map[string]any{"origin": "USER_DEFINED"},
			}
			policies[id] = policy
			json.NewEncoder(w).Encode(policy)
		case http.MethodDelete:
			delete(policies, id)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})
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
					resource "unifi_firewall_policy" "block_ssh" {
						name                = "Block Trusted to Servers SSH"
						action              = "BLOCK"
						source_zone_id      = "zone-trusted"
						destination_zone_id = "zone-servers"
						logging_enabled     = true

						destination_traffic_filter = {
							type = "PORT"
							port_filter = {
								items = [
									{ type = "PORT_NUMBER", value = 22 }
								]
							}
						}
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "name", "Block Trusted to Servers SSH"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "action", "BLOCK"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "logging_enabled", "true"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "destination_traffic_filter.type", "PORT"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "destination_traffic_filter.port_filter.items.0.type", "PORT_NUMBER"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "destination_traffic_filter.port_filter.items.0.value", "22"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.block_ssh", "destination_traffic_filter.port_filter.match_opposite", "false"),
				),
			},
		},
	})
}

func TestFirewallPolicy_basic(t *testing.T) {
	var mu sync.Mutex
	policies := map[string]map[string]any{}

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

	// Firewall policy endpoints — exact path for list/create
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Handle paginated list requests (GET with query params)
		if r.Method == http.MethodGet {
			var list []map[string]any
			for _, p := range policies {
				list = append(list, p)
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
			policy := map[string]any{
				"id":              "policy-test-id-001",
				"name":            req["name"],
				"enabled":         req["enabled"],
				"index":           0,
				"action":          req["action"],
				"source":          req["source"],
				"destination":     req["destination"],
				"ipProtocolScope": req["ipProtocolScope"],
				"loggingEnabled":  req["loggingEnabled"],
				"metadata": map[string]any{
					"origin": "USER_DEFINED",
				},
			}
			policies["policy-test-id-001"] = policy
			json.NewEncoder(w).Encode(policy)
			return
		}

		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Firewall policy endpoints — ID-specific paths (get, update, delete)
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/")

		switch r.Method {
		case http.MethodGet:
			policy, ok := policies[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"message": "not found"})
				return
			}
			json.NewEncoder(w).Encode(policy)

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)
			policy := map[string]any{
				"id":              id,
				"name":            req["name"],
				"enabled":         req["enabled"],
				"index":           0,
				"action":          req["action"],
				"source":          req["source"],
				"destination":     req["destination"],
				"ipProtocolScope": req["ipProtocolScope"],
				"loggingEnabled":  req["loggingEnabled"],
				"metadata": map[string]any{
					"origin": "USER_DEFINED",
				},
			}
			policies[id] = policy
			json.NewEncoder(w).Encode(policy)

		case http.MethodDelete:
			delete(policies, id)
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
					resource "unifi_firewall_policy" "test" {
						name                = "allow-lan-to-wan"
						action              = "ALLOW"
						allow_return_traffic = true
						source_zone_id      = "zone-src-001"
						destination_zone_id = "zone-dst-001"
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "name", "allow-lan-to-wan"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "action", "ALLOW"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "allow_return_traffic", "true"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "enabled", "true"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "ip_version", "IPV4"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "logging_enabled", "false"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "source_zone_id", "zone-src-001"),
					resource.TestCheckResourceAttr("unifi_firewall_policy.test", "destination_zone_id", "zone-dst-001"),
					resource.TestCheckResourceAttrSet("unifi_firewall_policy.test", "id"),
				),
			},
		},
	})
}
