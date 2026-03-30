package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ resource.Resource                = &FirewallPolicyResource{}
	_ resource.ResourceWithImportState = &FirewallPolicyResource{}
	_ resource.ResourceWithConfigure   = &FirewallPolicyResource{}
)

type FirewallPolicyResource struct {
	app *network.App
}

type FirewallPolicyModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	Index              types.Int64  `tfsdk:"index"`
	Action             types.String `tfsdk:"action"`
	AllowReturnTraffic types.Bool   `tfsdk:"allow_return_traffic"`
	SourceZoneID       types.String `tfsdk:"source_zone_id"`
	DestinationZoneID  types.String `tfsdk:"destination_zone_id"`
	IPVersion          types.String `tfsdk:"ip_version"`
	LoggingEnabled     types.Bool   `tfsdk:"logging_enabled"`
}

func NewFirewallPolicyResource() resource.Resource {
	return &FirewallPolicyResource{}
}

func (r *FirewallPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy"
}

func (r *FirewallPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a firewall policy (zone-pair rule).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Policy UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{Required: true},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"index":   schema.Int64Attribute{Computed: true, Description: "Evaluation index (read-only, managed via ordering endpoint)."},
			"action":  schema.StringAttribute{Required: true, Description: "ALLOW, BLOCK, or REJECT."},
			"allow_return_traffic": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Stateful return traffic (only for ALLOW actions).",
			},
			"source_zone_id":      schema.StringAttribute{Required: true, Description: "Source zone UUID."},
			"destination_zone_id": schema.StringAttribute{Required: true, Description: "Destination zone UUID."},
			"ip_version": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("IPV4"),
				Description: "IPV4, IPV6, or IPV4_AND_IPV6.",
			},
			"logging_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		},
	}
}

func (r *FirewallPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	p := req.ProviderData.(*UnifiProvider)
	r.app = network.NewApp(p.client, p.site)
}

func (r *FirewallPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiReq := policyModelToAPI(plan)
	result, err := r.app.CreateFirewallPolicy(ctx, apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall policy", err.Error())
		return
	}

	state := policyAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.app.GetFirewallPolicy(ctx, state.ID.ValueString())
	if err != nil {
		if unifiIsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall policy", err.Error())
		return
	}

	updated := policyAPIToModel(policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state FirewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiReq := policyModelToAPI(plan)
	result, err := r.app.UpdateFirewallPolicy(ctx, state.ID.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall policy", err.Error())
		return
	}

	updated := policyAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.app.DeleteFirewallPolicy(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall policy", err.Error())
	}
}

func (r *FirewallPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func policyModelToAPI(m FirewallPolicyModel) network.FirewallPolicyRequest {
	var req network.FirewallPolicyRequest
	req.Name = m.Name.ValueString()
	req.Enabled = boolOrDefault(m.Enabled, true)
	req.Action.Type = m.Action.ValueString()
	req.Action.AllowReturnTraffic = boolOrDefault(m.AllowReturnTraffic, false)
	req.Source.ZoneID = m.SourceZoneID.ValueString()
	req.Destination.ZoneID = m.DestinationZoneID.ValueString()
	req.IPProtocolScope.IPVersion = stringOrDefault(m.IPVersion, "IPV4")
	req.LoggingEnabled = boolOrDefault(m.LoggingEnabled, false)
	return req
}

func policyAPIToModel(p *network.FirewallPolicyResponse) FirewallPolicyModel {
	return FirewallPolicyModel{
		ID:                 stringValue(p.ID),
		Name:               stringValue(p.Name),
		Enabled:            boolValue(p.Enabled),
		Index:              int64Value(p.Index),
		Action:             stringValue(p.Action.Type),
		AllowReturnTraffic: boolValue(p.Action.AllowReturnTraffic),
		SourceZoneID:       stringValue(p.Source.ZoneID),
		DestinationZoneID:  stringValue(p.Destination.ZoneID),
		IPVersion:          stringValue(p.IPProtocolScope.IPVersion),
		LoggingEnabled:     boolValue(p.LoggingEnabled),
	}
}
