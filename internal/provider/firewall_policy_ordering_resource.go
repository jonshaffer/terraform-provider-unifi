package provider

import (
	"context"
	"fmt"
	"sort"
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
	ID                types.String `tfsdk:"id"`
	SourceZoneID      types.String `tfsdk:"source_zone_id"`
	DestinationZoneID types.String `tfsdk:"destination_zone_id"`
	OrderedPolicyIDs  types.List   `tfsdk:"ordered_policy_ids"`
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

	if err := r.applyOrdering(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Failed to set policy ordering", err.Error())
		return
	}

	plan.ID = types.StringValue(orderingID(plan.SourceZoneID.ValueString(), plan.DestinationZoneID.ValueString()))
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

	// Use Integration API GET (works fine) to read current ordering as UUIDs.
	ordering, err := r.app.GetPolicyOrdering(ctx, srcZone, dstZone)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read policy ordering", err.Error())
		return
	}

	policyIDs, _ := types.ListValueFrom(ctx, types.StringType, ordering.OrderedPolicyIDs.BeforeSystemDefined)
	state.OrderedPolicyIDs = policyIDs
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallPolicyOrderingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallPolicyOrderingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applyOrdering(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Failed to update policy ordering", err.Error())
		return
	}

	plan.ID = types.StringValue(orderingID(plan.SourceZoneID.ValueString(), plan.DestinationZoneID.ValueString()))
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

// applyOrdering translates Integration API UUIDs to v2 MongoDB ObjectIDs
// and calls the v2 batch-reorder endpoint (the only working write endpoint).
func (r *FirewallPolicyOrderingResource) applyOrdering(ctx context.Context, plan FirewallPolicyOrderingModel) error {
	policyUUIDs, d := expandStringList(ctx, plan.OrderedPolicyIDs)
	if d.HasError() {
		return fmt.Errorf("expanding policy IDs: %s", d.Errors())
	}

	srcZoneUUID := plan.SourceZoneID.ValueString()
	dstZoneUUID := plan.DestinationZoneID.ValueString()

	// Build UUID → MongoDB _id map by listing from both APIs and matching by name.
	integrationPolicies, err := r.app.ListFirewallPolicies(ctx)
	if err != nil {
		return fmt.Errorf("listing integration API policies: %w", err)
	}
	v2Policies, err := r.app.ListFirewallPoliciesV2(ctx)
	if err != nil {
		return fmt.Errorf("listing v2 policies: %w", err)
	}

	// Map Integration API name → UUID
	nameToUUID := make(map[string]string, len(integrationPolicies))
	for _, p := range integrationPolicies {
		nameToUUID[p.Name] = p.ID
	}

	// Map UUID → v2 _id (via name)
	uuidToInternalID := make(map[string]string)
	// Also collect zone UUID → v2 zone _id mapping from v2 policy data.
	zoneUUIDToInternalID := make(map[string]string)
	for _, v2p := range v2Policies {
		if uuid, ok := nameToUUID[v2p.Name]; ok {
			uuidToInternalID[uuid] = v2p.ID

			// The v2 policy includes zone _ids in source/destination.
			// Find the matching Integration API policy to get zone UUIDs.
			for _, ip := range integrationPolicies {
				if ip.Name == v2p.Name {
					if v2p.Source.ZoneID != "" {
						zoneUUIDToInternalID[ip.Source.ZoneID] = v2p.Source.ZoneID
					}
					if v2p.Destination.ZoneID != "" {
						zoneUUIDToInternalID[ip.Destination.ZoneID] = v2p.Destination.ZoneID
					}
					break
				}
			}
		}
	}

	// Translate zone UUIDs to v2 _ids.
	srcZoneInternal, ok := zoneUUIDToInternalID[srcZoneUUID]
	if !ok {
		return fmt.Errorf("could not find v2 internal ID for source zone %s", srcZoneUUID)
	}
	dstZoneInternal, ok := zoneUUIDToInternalID[dstZoneUUID]
	if !ok {
		return fmt.Errorf("could not find v2 internal ID for destination zone %s", dstZoneUUID)
	}

	// Translate policy UUIDs to v2 _ids in order.
	internalIDs := make([]string, len(policyUUIDs))
	for i, uuid := range policyUUIDs {
		internalID, ok := uuidToInternalID[uuid]
		if !ok {
			return fmt.Errorf("could not find v2 internal ID for policy %s", uuid)
		}
		internalIDs[i] = internalID
	}

	// All user-defined policies go in before_predefined_ids.
	return r.app.BatchReorderPolicies(ctx, network.BatchReorderRequest{
		BeforePredefinedIDs: internalIDs,
		AfterPredefinedIDs:  []string{},
		SourceZoneID:        srcZoneInternal,
		DestinationZoneID:   dstZoneInternal,
	})
}

func orderingID(srcZoneID, dstZoneID string) string {
	return srcZoneID + "/" + dstZoneID
}

// Ensure sort is available (used for potential future ordering verification).
var _ = sort.Strings
