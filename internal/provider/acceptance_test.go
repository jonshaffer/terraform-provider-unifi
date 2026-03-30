// Acceptance tests for the UniFi Terraform provider.
// These tests require a live UniFi controller and are gated on TF_ACC=1.
//
// Required environment variables:
//   - TF_ACC=1 (enables acceptance tests)
//   - UNIFI_API_KEY (API key for the controller)
//   - UNIFI_BASE_URL (e.g., https://192.168.1.1)
//   - UNIFI_SITE (optional, defaults to "default")
package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var accProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"unifi": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("Set TF_ACC=1 to run acceptance tests")
	}
	if os.Getenv("UNIFI_API_KEY") == "" {
		t.Fatal("UNIFI_API_KEY must be set for acceptance tests")
	}
	if os.Getenv("UNIFI_BASE_URL") == "" {
		t.Fatal("UNIFI_BASE_URL must be set for acceptance tests")
	}
}

func TestAccDNSRecord_lifecycle(t *testing.T) {
	testAccPreCheck(t)

	testKey := fmt.Sprintf("acc-test-%d.hyperfluid.dev", os.Getpid())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: accProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: fmt.Sprintf(`
					resource "unifi_dns_record" "test" {
						key         = %q
						value       = "192.168.99.99"
						record_type = "A"
					}
				`, testKey),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_dns_record.test", "key", testKey),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "value", "192.168.99.99"),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "record_type", "A"),
					resource.TestCheckResourceAttr("unifi_dns_record.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("unifi_dns_record.test", "id"),
				),
			},
			// Update value
			{
				Config: fmt.Sprintf(`
					resource "unifi_dns_record" "test" {
						key         = %q
						value       = "192.168.99.100"
						record_type = "A"
					}
				`, testKey),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("unifi_dns_record.test", "value", "192.168.99.100"),
				),
			},
			// Import
			{
				ResourceName:      "unifi_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
