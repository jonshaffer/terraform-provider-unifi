package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi"
)

var _ provider.Provider = &UnifiProvider{}

type UnifiProvider struct {
	version      string
	client       *unifi.Client
	site         string
	capabilities *unifi.Capabilities
}

type UnifiProviderModel struct {
	APIKey   types.String `tfsdk:"api_key"`
	BaseURL  types.String `tfsdk:"base_url"`
	Site     types.String `tfsdk:"site"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UnifiProvider{
			version: version,
		}
	}
}

func (p *UnifiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "unifi"
	resp.Version = p.version
}

func (p *UnifiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage UniFi network infrastructure declaratively.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "UniFi API key. Can also be set via UNIFI_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"base_url": schema.StringAttribute{
				Description: "UniFi controller base URL (e.g., https://192.168.1.1). Can also be set via UNIFI_BASE_URL.",
				Optional:    true,
			},
			"site": schema.StringAttribute{
				Description: "UniFi site name. Defaults to 'default'. Can also be set via UNIFI_SITE.",
				Optional:    true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Defaults to true for self-signed certs.",
				Optional:    true,
			},
		},
	}
}

func (p *UnifiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config UnifiProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := envOrValue(config.APIKey, "UNIFI_API_KEY")
	baseURL := envOrValue(config.BaseURL, "UNIFI_BASE_URL")
	site := envOrValue(config.Site, "UNIFI_SITE")
	insecure := true
	if !config.Insecure.IsNull() {
		insecure = config.Insecure.ValueBool()
	}

	if apiKey == "" {
		resp.Diagnostics.AddError("Missing API Key", "api_key must be set in provider config or UNIFI_API_KEY environment variable")
		return
	}
	if baseURL == "" {
		resp.Diagnostics.AddError("Missing Base URL", "base_url must be set in provider config or UNIFI_BASE_URL environment variable")
		return
	}
	if site == "" {
		site = "default"
	}

	client, err := unifi.NewClient(unifi.ClientConfig{
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Insecure: insecure,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create UniFi client", err.Error())
		return
	}

	// Feature detection (FR-017): discover controller capabilities at configure time
	caps, err := unifi.DetectCapabilities(client, ctx)
	if err != nil {
		resp.Diagnostics.AddWarning("Feature detection failed", err.Error())
	} else {
		if valErr := unifi.ValidateCapabilities(caps); valErr != nil {
			resp.Diagnostics.AddWarning("Controller capability check", valErr.Error())
		}
	}

	p.client = client
	p.site = site
	p.capabilities = caps

	resp.DataSourceData = p
	resp.ResourceData = p
}

func envOrValue(v types.String, envVar string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(envVar)
}

func (p *UnifiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDNSRecordResource,
		NewNetworkResource,
	}
}

func (p *UnifiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDNSRecordDataSource,
		NewNetworkDataSource,
	}
}
