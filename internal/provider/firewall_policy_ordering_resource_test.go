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
	// Track ordering per zone pair (keyed by "srcZone/dstZone")
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

	// Policy ordering endpoint
	mux.HandleFunc("/proxy/network/integration/v1/sites/test-site-uuid/firewall/policies/ordering", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		srcZone := r.URL.Query().Get("sourceFirewallZoneId")
		dstZone := r.URL.Query().Get("destinationFirewallZoneId")
		key := srcZone + "/" + dstZone

		switch r.Method {
		case http.MethodGet:
			ids, ok := orderings[key]
			if !ok {
				ids = []string{}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"orderedFirewallPolicyIds": map[string]any{
					"beforeSystemDefined": ids,
					"afterSystemDefined":  []string{},
				},
			})

		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			json.Unmarshal(body, &req)

			if outer, ok := req["orderedFirewallPolicyIds"].(map[string]any); ok {
				if before, ok := outer["beforeSystemDefined"].([]any); ok {
					ids := make([]string, len(before))
					for i, id := range before {
						ids[i], _ = id.(string)
					}
					orderings[key] = ids
				}
			}
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
			// Create ordering
			{
				Config: `
					resource "unifi_firewall_policy_ordering" "test" {
						source_zone_id      = "zone-trusted"
						destination_zone_id = "zone-servers"
						ordered_policy_ids  = ["policy-ssh-block", "policy-allow-all"]
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "id", "zone-trusted/zone-servers"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "source_zone_id", "zone-trusted"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "destination_zone_id", "zone-servers"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.#", "2"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.0", "policy-ssh-block"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.1", "policy-allow-all"),
				),
			},
			// Update ordering (reorder + add a policy)
			{
				Config: `
					resource "unifi_firewall_policy_ordering" "test" {
						source_zone_id      = "zone-trusted"
						destination_zone_id = "zone-servers"
						ordered_policy_ids  = ["policy-allow-all", "policy-ssh-block", "policy-new"]
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.#", "3"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.0", "policy-allow-all"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.1", "policy-ssh-block"),
					resource.TestCheckResourceAttr("unifi_firewall_policy_ordering.test", "ordered_policy_ids.2", "policy-new"),
				),
			},
			// Import
			{
				ResourceName:      "unifi_firewall_policy_ordering.test",
				ImportState:       true,
				ImportStateId:     "zone-trusted/zone-servers",
				ImportStateVerify: true,
			},
		},
	})
}
