package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ resource.Resource                = &NetworkResource{}
	_ resource.ResourceWithImportState = &NetworkResource{}
	_ resource.ResourceWithConfigure   = &NetworkResource{}
)

type NetworkResource struct {
	app *network.App
}

type NetworkModel struct {
	ID                      types.String `tfsdk:"id"`
	ExternalID              types.String `tfsdk:"external_id"`
	Name                    types.String `tfsdk:"name"`
	Purpose                 types.String `tfsdk:"purpose"`
	VLAN                    types.Int64  `tfsdk:"vlan"`
	VLANEnabled             types.Bool   `tfsdk:"vlan_enabled"`
	IPSubnet                types.String `tfsdk:"ip_subnet"`
	DhcpdEnabled            types.Bool   `tfsdk:"dhcpd_enabled"`
	DhcpdStart              types.String `tfsdk:"dhcpd_start"`
	DhcpdStop               types.String `tfsdk:"dhcpd_stop"`
	MdnsEnabled             types.Bool   `tfsdk:"mdns_enabled"`
	InternetAccessEnabled   types.Bool   `tfsdk:"internet_access_enabled"`
	NetworkIsolationEnabled types.Bool   `tfsdk:"network_isolation_enabled"`
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

func (r *NetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a network (VLAN) on the UniFi controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Legacy _id (24-char hex).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"external_id": schema.StringAttribute{
				Computed: true, Description: "Zone API UUID for firewall zone references.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":         schema.StringAttribute{Required: true},
			"purpose":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"vlan":         schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64RequiresReplace()}},
			"vlan_enabled":  schema.BoolAttribute{Required: true, PlanModifiers: []planmodifier.Bool{boolRequiresReplace()}},
			"ip_subnet":    schema.StringAttribute{Required: true},
			"dhcpd_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"dhcpd_start":  schema.StringAttribute{Optional: true, Computed: true},
			"dhcpd_stop":   schema.StringAttribute{Optional: true, Computed: true},
			"mdns_enabled":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"internet_access_enabled":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"network_isolation_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		},
	}
}

func (r *NetworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p, ok := req.ProviderData.(*UnifiProvider)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.app = network.NewApp(p.client, p.site)
}

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.app.CreateNetwork(ctx, networkModelToAPI(plan))
	if err != nil {
		resp.Diagnostics.AddError("Failed to create network", err.Error())
		return
	}

	state := networkAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	net, err := r.app.GetNetwork(ctx, state.ID.ValueString())
	if err != nil {
		if unifiIsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read network", err.Error())
		return
	}

	updated := networkAPIToModel(net)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state NetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiNet := networkModelToAPI(plan)
	apiNet.ID = state.ID.ValueString()

	result, err := r.app.UpdateNetwork(ctx, apiNet)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update network", err.Error())
		return
	}

	updated := networkAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.app.DeleteNetwork(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete network", err.Error())
	}
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func networkModelToAPI(m NetworkModel) network.Network {
	return network.Network{
		Name:                    m.Name.ValueString(),
		Purpose:                 m.Purpose.ValueString(),
		VLAN:                    int64OrDefault(m.VLAN, 0),
		VLANEnabled:             boolOrDefault(m.VLANEnabled, false),
		IPSubnet:                m.IPSubnet.ValueString(),
		DhcpdEnabled:            boolOrDefault(m.DhcpdEnabled, false),
		DhcpdStart:              stringOrDefault(m.DhcpdStart, ""),
		DhcpdStop:               stringOrDefault(m.DhcpdStop, ""),
		MdnsEnabled:             boolOrDefault(m.MdnsEnabled, false),
		InternetAccessEnabled:   boolOrDefault(m.InternetAccessEnabled, true),
		NetworkIsolationEnabled: boolOrDefault(m.NetworkIsolationEnabled, false),
	}
}

func networkAPIToModel(n *network.Network) NetworkModel {
	return NetworkModel{
		ID:                      stringValue(n.ID),
		ExternalID:              stringValueOrNull(n.ExternalID),
		Name:                    stringValue(n.Name),
		Purpose:                 stringValue(n.Purpose),
		VLAN:                    int64Value(n.VLAN),
		VLANEnabled:             boolValue(n.VLANEnabled),
		IPSubnet:                stringValue(n.IPSubnet),
		DhcpdEnabled:            boolValue(n.DhcpdEnabled),
		DhcpdStart:              stringValueOrNull(n.DhcpdStart),
		DhcpdStop:               stringValueOrNull(n.DhcpdStop),
		MdnsEnabled:             boolValue(n.MdnsEnabled),
		InternetAccessEnabled:   boolValue(n.InternetAccessEnabled),
		NetworkIsolationEnabled: boolValue(n.NetworkIsolationEnabled),
	}
}
