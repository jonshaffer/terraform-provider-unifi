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
	_ datasource.DataSource              = &NetworkDataSource{}
	_ datasource.DataSourceWithConfigure = &NetworkDataSource{}
)

type NetworkDataSource struct {
	app *network.App
}

type NetworkDataSourceModel struct {
	ID                      types.String `tfsdk:"id"`
	ExternalID              types.String `tfsdk:"external_id"`
	Name                    types.String `tfsdk:"name"`
	Purpose                 types.String `tfsdk:"purpose"`
	VLAN                    types.Int64  `tfsdk:"vlan"`
	VLANEnabled             types.Bool   `tfsdk:"vlan_enabled"`
	IPSubnet                types.String `tfsdk:"ip_subnet"`
	MdnsEnabled             types.Bool   `tfsdk:"mdns_enabled"`
	InternetAccessEnabled   types.Bool   `tfsdk:"internet_access_enabled"`
}

func NewNetworkDataSource() datasource.DataSource {
	return &NetworkDataSource{}
}

func (d *NetworkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (d *NetworkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a network by name.",
		Attributes: map[string]schema.Attribute{
			"id":                      schema.StringAttribute{Computed: true},
			"external_id":             schema.StringAttribute{Computed: true},
			"name":                    schema.StringAttribute{Required: true, Description: "Network name to look up."},
			"purpose":                 schema.StringAttribute{Computed: true},
			"vlan":                    schema.Int64Attribute{Computed: true},
			"vlan_enabled":            schema.BoolAttribute{Computed: true},
			"ip_subnet":              schema.StringAttribute{Computed: true},
			"mdns_enabled":            schema.BoolAttribute{Computed: true},
			"internet_access_enabled": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *NetworkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p, ok := req.ProviderData.(*UnifiProvider)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	d.app = network.NewApp(p.client, p.site)
}

func (d *NetworkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NetworkDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nets, err := d.app.ListNetworks(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list networks", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, n := range nets {
		if n.Name == name {
			config.ID = stringValue(n.ID)
			config.ExternalID = stringValueOrNull(n.ExternalID)
			config.Purpose = stringValue(n.Purpose)
			config.VLAN = int64Value(n.VLAN)
			config.VLANEnabled = boolValue(n.VLANEnabled)
			config.IPSubnet = stringValue(n.IPSubnet)
			config.MdnsEnabled = boolValue(n.MdnsEnabled)
			config.InternetAccessEnabled = boolValue(n.InternetAccessEnabled)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError("Network not found", fmt.Sprintf("no network named %q", name))
}
