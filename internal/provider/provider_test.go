package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testProtoV6ProviderFactories returns provider factories for unit tests.
// The provider reads configuration from environment variables:
//   - UNIFI_API_KEY: set to "test-key" in tests
//   - UNIFI_BASE_URL: set to the mock server URL in tests
//   - UNIFI_SITE: set to "default" in tests
func testProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"unifi": providerserver.NewProtocol6WithError(New("test")()),
	}
}
