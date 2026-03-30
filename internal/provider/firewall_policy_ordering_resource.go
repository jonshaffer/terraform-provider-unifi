package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ resource.Resource                = &FirewallPolicyOrderingResource{}
	_ resource.ResourceWithImportState = &FirewallPolicyOrderingResource{}
	_ resource.ResourceWithConfigure   = &FirewallPolicyOrderingResource{}
)

type FirewallPolicyOrderingResource struct {
	app *network.App
}

type FirewallPolicyOrderingModel struct {
	ID               types.String `tfsdk:"id"`
	SourceZoneID     types.String `tfsdk:"source_zone_id"`
	DestinationZoneID types.String `tfsdk:"destination_zone_id"`
	OrderedPolicyIDs types.List   `tfsdk:"ordered_policy_ids"`
}

func NewFirewallPolicyOrderingResource() resource.Resource {
	return &FirewallPolicyOrderingResource{}
}

func (r *FirewallPolicyOrderingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy_ordering"
}

func (r *FirewallPolicyOrderingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the evaluation order of firewall policies for a zone pair.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite ID (source_zone_id/destination_zone_id).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"source_zone_id": schema.StringAttribute{
				Required:    true,
				Description: "Source firewall zone UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"destination_zone_id": schema.StringAttribute{
				Required:    true,
				Description: "Destination firewall zone UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ordered_policy_ids": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Ordered list of firewall policy UUIDs. First element evaluates first.",
			},
		},
	}
}

func (r *FirewallPolicyOrderingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p := req.ProviderData.(*UnifiProvider)
	r.app = network.NewApp(p.client, p.site)
}

func (r *FirewallPolicyOrderingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyIDs, d := expandStringList(ctx, plan.OrderedPolicyIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	srcZone := plan.SourceZoneID.ValueString()
	dstZone := plan.DestinationZoneID.ValueString()

	err := r.app.SetPolicyOrdering(ctx, srcZone, dstZone, network.PolicyOrdering{
		OrderedPolicyIDs: policyIDs,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to set policy ordering", err.Error())
		return
	}

	plan.ID = types.StringValue(orderingID(srcZone, dstZone))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallPolicyOrderingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	srcZone := state.SourceZoneID.ValueString()
	dstZone := state.DestinationZoneID.ValueString()

	ordering, err := r.app.GetPolicyOrdering(ctx, srcZone, dstZone)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read policy ordering", err.Error())
		return
	}

	policyIDs, _ := types.ListValueFrom(ctx, types.StringType, ordering.OrderedPolicyIDs)
	state.OrderedPolicyIDs = policyIDs
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallPolicyOrderingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyIDs, d := expandStringList(ctx, plan.OrderedPolicyIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	srcZone := plan.SourceZoneID.ValueString()
	dstZone := plan.DestinationZoneID.ValueString()

	err := r.app.SetPolicyOrdering(ctx, srcZone, dstZone, network.PolicyOrdering{
		OrderedPolicyIDs: policyIDs,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update policy ordering", err.Error())
		return
	}

	plan.ID = types.StringValue(orderingID(srcZone, dstZone))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirewallPolicyOrderingResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Ordering is inherent to the zone pair — there is no API to "delete" it.
	// Removing from state is sufficient; the controller retains its current ordering.
}

func (r *FirewallPolicyOrderingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected format: source_zone_id/destination_zone_id, got: %s", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("source_zone_id"), types.StringValue(parts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("destination_zone_id"), types.StringValue(parts[1]))...)
}

func orderingID(srcZoneID, dstZoneID string) string {
	return srcZoneID + "/" + dstZoneID
}
