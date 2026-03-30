package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ resource.Resource                = &FirewallZoneResource{}
	_ resource.ResourceWithImportState = &FirewallZoneResource{}
	_ resource.ResourceWithConfigure   = &FirewallZoneResource{}
)

type FirewallZoneResource struct {
	app *network.App
}

type FirewallZoneModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	NetworkIDs   types.List   `tfsdk:"network_ids"`
	Origin       types.String `tfsdk:"origin"`
	Configurable types.Bool   `tfsdk:"configurable"`
}

func NewFirewallZoneResource() resource.Resource {
	return &FirewallZoneResource{}
}

func (r *FirewallZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_zone"
}

func (r *FirewallZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a firewall zone. System-defined zones cannot be deleted.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Zone UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":       schema.StringAttribute{Required: true},
			"network_ids": schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Network UUIDs (external_id) assigned to this zone."},
			"origin":      schema.StringAttribute{Computed: true, Description: "SYSTEM_DEFINED or USER_DEFINED."},
			"configurable": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *FirewallZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p := req.ProviderData.(*UnifiProvider)
	r.app = network.NewApp(p.client, p.site)
}

func (r *FirewallZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkIDs, d := expandStringList(ctx, plan.NetworkIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.app.CreateFirewallZone(ctx, network.FirewallZoneRequest{
		Name: plan.Name.ValueString(), NetworkIDs: networkIDs,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall zone", err.Error())
		return
	}

	state := zoneAPIToModel(zone)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.app.GetFirewallZone(ctx, state.ID.ValueString())
	if err != nil {
		if unifiIsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall zone", err.Error())
		return
	}

	updated := zoneAPIToModel(zone)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkIDs, d := expandStringList(ctx, plan.NetworkIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.app.UpdateFirewallZone(ctx, state.ID.ValueString(), network.FirewallZoneRequest{
		Name: plan.Name.ValueString(), NetworkIDs: networkIDs,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall zone", err.Error())
		return
	}

	updated := zoneAPIToModel(zone)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Origin.ValueString() == "SYSTEM_DEFINED" {
		resp.Diagnostics.AddError("Cannot delete system-defined zone",
			fmt.Sprintf("Zone %q is system-defined and cannot be deleted", state.Name.ValueString()))
		return
	}

	if err := r.app.DeleteFirewallZone(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall zone", err.Error())
	}
}

func (r *FirewallZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func zoneAPIToModel(z *network.FirewallZoneResponse) FirewallZoneModel {
	networkIDs, _ := types.ListValueFrom(context.Background(), types.StringType, z.NetworkIDs)
	return FirewallZoneModel{
		ID:           stringValue(z.ID),
		Name:         stringValue(z.Name),
		NetworkIDs:   networkIDs,
		Origin:       stringValue(z.Metadata.Origin),
		Configurable: boolValue(z.Metadata.Configurable),
	}
}
