package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestFirewallPolicyOrdering_basic(t *testing.T) {
	var mu sync.Mutex
	// Track ordering per zone pair (keyed by "srcZone/dstZone" using v2 internal IDs)
	orderings := map[string][]string{}

	mux := http.NewServeMux()

	// Feature detection stubs
	mux.HandleFunc("/proxy/network/api-docs/integration.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"info": map[string]any{"version": "1.0.0"}, "paths": map[string]any{}})
	})
	mux.HandleFunc("/proxy/network/v2/api/site/default/features", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{})
	})
	mux.HandleFunc("/proxy/network/integration/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "test-site-uuid"}}})
	})

	// Integration API: list policies (returns UUIDs)
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id": "uuid-ssh-block", "name": "Block SSH", "enabled": true, "index": 10000,
					"action": map[string]any{"type": "BLOCK"}, "loggingEnabled": true,
					"source":          map[string]any{"zoneId": "zone-trusted-uuid"},
					"destination":     map[string]any{"zoneId": "zone-servers-uuid"},
					"ipProtocolScope": map[string]any{"ipVersion": "IPV4"},
					"metadata":        map[string]any{"origin": "USER_DEFINED"},
				},
				{
					"id": "uuid-allow-all", "name": "Allow Trusted to Servers", "enabled": true, "index": 40000,
					"action": map[string]any{"type": "ALLOW", "allowReturnTraffic": true}, "loggingEnabled": false,
					"source":          map[string]any{"zoneId": "zone-trusted-uuid"},
					"destination":     map[string]any{"zoneId": "zone-servers-uuid"},
					"ipProtocolScope": map[string]any{"ipVersion": "IPV4"},
					"metadata":        map[string]any{"origin": "USER_DEFINED"},
				},
			},
			"offset": 0, "limit": 200, "count": 2, "totalCount": 2,
		})
	})

	// Integration API: GET ordering (returns UUIDs)
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/ordering", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		// Return ordering based on what batch-reorder set, mapped back to UUIDs
		key := "mongo-zone-trusted/mongo-zone-servers"
		ids, ok := orderings[key]
		if !ok {
			ids = []string{}
		}
		// Map internal IDs back to UUIDs for the Integration API response
		internalToUUID := map[string]string{
			"mongo-ssh-block": "uuid-ssh-block",
			"mongo-allow-all": "uuid-allow-all",
		}
		uuids := make([]string, len(ids))
		for i, id := range ids {
			uuids[i] = internalToUUID[id]
		}
		json.NewEncoder(w).Encode(map[string]any{
			"orderedFirewallPolicyIds": map[string]any{
				"beforeSystemDefined": uuids,
				"afterSystemDefined":  []string{},
			},
		})
	})

	// v2 API: batch-reorder (must register BEFORE the list handler due to ServeMux matching)
	mux.HandleFunc("/proxy/network/v2/api/site/default/firewall-policies/batch-reorder", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		srcZone, _ := req["source_zone_id"].(string)
		dstZone, _ := req["destination_zone_id"].(string)
		key := srcZone + "/" + dstZone

		if before, ok := req["before_predefined_ids"].([]any); ok {
			ids := make([]string, len(before))
			for i, id := range before {
				ids[i], _ = id.(string)
			}
			orderings[key] = ids
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	// v2 API: list policies (returns MongoDB _ids)
	mux.HandleFunc("/proxy/network/v2/api/site/default/firewall-policies", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"_id": "mongo-ssh-block", "name": "Block SSH", "index": 10000,
				"source":      map[string]any{"zone_id": "mongo-zone-trusted"},
				"destination": map[string]any{"zone_id": "mongo-zone-servers"},
			},
			{
				"_id": "mongo-allow-all", "name": "Allow Trusted to Servers", "index": 40000,
				"source":      map[string]any{"zone_id": "mongo-zone-trusted"},
				"destination": map[string]any{"zone_id": "mongo-zone-servers"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("UNIFI_API_KEY", "test-key")
	t.Setenv("UNIFI_BASE_URL", srv.URL)
	t.Setenv("UNIFI_SITE", "default")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			// Create ordering (SSH block first)
			{
				Config: `
					resource "unifi_firewall_policy_ordering" "test" {
						source_zone_id      = "zone-trusted-uuid"
						destination_zone_id = "zone-servers-uuid"
						ordered_policy_ids  = ["uuid-ssh-block", "uuid-allow-all"]
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "id", "zone-trusted-uuid/zone-servers-uuid"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.#", "2"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.0", "uuid-ssh-block"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.1", "uuid-allow-all"),
				),
			},
		},
	})
}
