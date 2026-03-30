package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jonshaffer/go-unifi/unifi/network"
)

var (
	_ resource.Resource                = &FirewallGroupResource{}
	_ resource.ResourceWithImportState = &FirewallGroupResource{}
	_ resource.ResourceWithConfigure   = &FirewallGroupResource{}
)

type FirewallGroupResource struct {
	app *network.App
}

type FirewallGroupModel struct {
	ID         types.String `tfsdk:"id"`
	ExternalID types.String `tfsdk:"external_id"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Members    types.List   `tfsdk:"members"`
}

func NewFirewallGroupResource() resource.Resource {
	return &FirewallGroupResource{}
}

func (r *FirewallGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_group"
}

func (r *FirewallGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a firewall group (IP address group or port group).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Legacy _id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"external_id": schema.StringAttribute{
				Computed: true, Description: "UUID used by policy traffic filters as trafficMatchingListId.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true},
			"type": schema.StringAttribute{
				Required: true, Description: "address-group or port-group.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"members": schema.ListAttribute{
				Required: true, Description: "IPs/CIDRs (address-group) or port numbers/ranges (port-group).",
				ElementType: types.StringType,
			},
		},
	}
}

func (r *FirewallGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirewallGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirewallGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, memberDiags := expandStringList(ctx, plan.Members)
	resp.Diagnostics.Append(memberDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.app.CreateFirewallGroup(ctx, network.FirewallGroup{
		Name:         plan.Name.ValueString(),
		GroupType:    plan.Type.ValueString(),
		GroupMembers: members,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create firewall group", err.Error())
		return
	}

	state := firewallGroupAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *FirewallGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirewallGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.app.GetFirewallGroup(ctx, state.ID.ValueString())
	if err != nil {
		if unifiIsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read firewall group", err.Error())
		return
	}

	updated := firewallGroupAPIToModel(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirewallGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FirewallGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, memberDiags := expandStringList(ctx, plan.Members)
	resp.Diagnostics.Append(memberDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.app.UpdateFirewallGroup(ctx, network.FirewallGroup{
		ID:           state.ID.ValueString(),
		Name:         plan.Name.ValueString(),
		GroupType:    plan.Type.ValueString(),
		GroupMembers: members,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update firewall group", err.Error())
		return
	}

	updated := firewallGroupAPIToModel(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)
}

func (r *FirewallGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FirewallGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.app.DeleteFirewallGroup(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete firewall group", err.Error())
	}
}

func (r *FirewallGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func firewallGroupAPIToModel(g *network.FirewallGroup) FirewallGroupModel {
	members, _ := types.ListValueFrom(context.Background(), types.StringType, g.GroupMembers)
	return FirewallGroupModel{
		ID:         stringValue(g.ID),
		ExternalID: stringValueOrNull(g.ExternalID),
		Name:       stringValue(g.Name),
		Type:       stringValue(g.GroupType),
		Members:    members,
	}
}

func expandStringList(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	var result []string
	diags := list.ElementsAs(ctx, &result, false)
	return result, diags
}
