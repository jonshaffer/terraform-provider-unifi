package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ datasource.DataSource              = &FirewallZoneDataSource{}
	_ datasource.DataSourceWithConfigure = &FirewallZoneDataSource{}
)

type FirewallZoneDataSource struct {
	app *network.App
}

type FirewallZoneDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Origin       types.String `tfsdk:"origin"`
	Configurable types.Bool   `tfsdk:"configurable"`
}

func NewFirewallZoneDataSource() datasource.DataSource {
	return &FirewallZoneDataSource{}
}

func (d *FirewallZoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_zone"
}

func (d *FirewallZoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a firewall zone by name. Use for system-defined zones (Gateway, External, VPN, Hotspot).",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Computed: true},
			"name":         schema.StringAttribute{Required: true, Description: "Zone name to look up."},
			"origin":       schema.StringAttribute{Computed: true},
			"configurable": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *FirewallZoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p := req.ProviderData.(*UnifiProvider)
	d.app = network.NewApp(p.client, p.site)
}

func (d *FirewallZoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FirewallZoneDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zones, err := d.app.ListFirewallZones(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list firewall zones", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, z := range zones {
		if z.Name == name {
			config.ID = stringValue(z.ID)
			config.Origin = stringValue(z.Metadata.Origin)
			config.Configurable = boolValue(z.Metadata.Configurable)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError("Firewall zone not found", fmt.Sprintf("no zone named %q", name))
}
