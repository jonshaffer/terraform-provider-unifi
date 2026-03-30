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

func TestDNSRecord_basic(t *testing.T) {
	var mu sync.Mutex
	records := map[string]map[string]any{}

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

	// DNS record endpoints — exact path match for list/create
	mux.HandleFunc("/proxy/network/v2/api/site/default/static-dns", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			// List all records (used by GetDNSRecord which lists then filters)
			var list []map[string]any
			for _, rec := range records {
				list = append(list, rec)
			}
			json.NewEncoder(w).Encode(list)

		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var rec map[string]any
			json.Unmarshal(body, &rec)
			rec["_id"] = "dns-test-id-001"
			records["dns-test-id-001"] = rec
			json.NewEncoder(w).Encode(rec)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// DNS record endpoints — ID-specific paths (update, delete)
	mux.HandleFunc("/proxy/network/v2/api/site/default/static-dns/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		id := strings.TrimPrefix(r.URL.Path, "/proxy/network/v2/api/site/default/static-dns/")

		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var rec map[string]any
			json.Unmarshal(body, &rec)
			rec["_id"] = id
			records[id] = rec
			json.NewEncoder(w).Encode(rec)

		case http.MethodDelete:
			delete(records, id)
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
					resource "unifi_dns_record" "test" {
						key         = "test.example.com"
						value       = "192.168.1.100"
						record_type = "A"
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_dns_record.test", "key", "test.example.com"),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "value", "192.168.1.100"),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "record_type", "A"),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("unifi_dns_record.test", "id"),
				),
			},
			// ImportState test
			{
				ResourceName:      "unifi_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
