package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	ID                       types.String        `tfsdk:"id"`
	Name                     types.String        `tfsdk:"name"`
	Enabled                  types.Bool          `tfsdk:"enabled"`
	Index                    types.Int64         `tfsdk:"index"`
	Action                   types.String        `tfsdk:"action"`
	AllowReturnTraffic       types.Bool          `tfsdk:"allow_return_traffic"`
	SourceZoneID             types.String        `tfsdk:"source_zone_id"`
	DestinationZoneID        types.String        `tfsdk:"destination_zone_id"`
	IPVersion                types.String        `tfsdk:"ip_version"`
	LoggingEnabled           types.Bool          `tfsdk:"logging_enabled"`
	DestinationTrafficFilter *TrafficFilterModel `tfsdk:"destination_traffic_filter"`
	SourceTrafficFilter      *TrafficFilterModel `tfsdk:"source_traffic_filter"`
}

// TrafficFilterModel represents a source or destination traffic filter.
// The type field discriminates which sub-filter is active:
//   - PORT: only port_filter is used
//   - NETWORK: network_filter (and optionally port_filter) are used
//   - IP_ADDRESS: ip_address_filter (and optionally port_filter) are used
type TrafficFilterModel struct {
	Type            types.String          `tfsdk:"type"`
	PortFilter      *PortFilterModel      `tfsdk:"port_filter"`
	NetworkFilter   *NetworkFilterModel   `tfsdk:"network_filter"`
	IPAddressFilter *IPAddressFilterModel `tfsdk:"ip_address_filter"`
}

type PortFilterModel struct {
	MatchOpposite types.Bool            `tfsdk:"match_opposite"`
	Items         []PortFilterItemModel `tfsdk:"items"`
}

type PortFilterItemModel struct {
	Type  types.String `tfsdk:"type"`
	Value types.Int64  `tfsdk:"value"`
}

type NetworkFilterModel struct {
	NetworkIDs    types.List `tfsdk:"network_ids"`
	MatchOpposite types.Bool `tfsdk:"match_opposite"`
}

type IPAddressFilterModel struct {
	Type                  types.String         `tfsdk:"type"`
	MatchOpposite         types.Bool           `tfsdk:"match_opposite"`
	TrafficMatchingListID types.String         `tfsdk:"traffic_matching_list_id"`
	Items                 []IPAddressItemModel `tfsdk:"items"`
}

type IPAddressItemModel struct {
	Type  types.String `tfsdk:"type"`
	Value types.String `tfsdk:"value"`
}

func NewFirewallPolicyResource() resource.Resource {
	return &FirewallPolicyResource{}
}

func (r *FirewallPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy"
}

// trafficFilterSchemaAttr returns the schema for a traffic filter as an optional nested attribute.
func trafficFilterSchemaAttr(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: description,
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Filter type: PORT, NETWORK, or IP_ADDRESS.",
			},
			"port_filter": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Port-based traffic filter.",
				Attributes: map[string]schema.Attribute{
					"match_opposite": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
						Description: "Match traffic NOT matching the filter.",
					},
					"items": schema.ListNestedAttribute{
						Required:    true,
						Description: "Port items to match.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									Required:    true,
									Description: "PORT_NUMBER or PORT_RANGE.",
								},
								"value": schema.Int64Attribute{
									Required:    true,
									Description: "Port number (e.g. 22, 53, 123).",
								},
							},
						},
					},
				},
			},
			"network_filter": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Network-based traffic filter. Used when type is NETWORK.",
				Attributes: map[string]schema.Attribute{
					"network_ids": schema.ListAttribute{
						Required:    true,
						ElementType: types.StringType,
						Description: "Network UUIDs to match.",
					},
					"match_opposite": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
					},
				},
			},
			"ip_address_filter": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "IP address-based traffic filter. Used when type is IP_ADDRESS.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Optional:    true,
						Description: "IP_ADDRESSES or TRAFFIC_MATCHING_LIST.",
					},
					"match_opposite": schema.BoolAttribute{
						Optional: true, Computed: true, Default: booldefault.StaticBool(false),
					},
					"traffic_matching_list_id": schema.StringAttribute{
						Optional:    true,
						Description: "Firewall group UUID (for TRAFFIC_MATCHING_LIST type).",
					},
					"items": schema.ListNestedAttribute{
						Optional:    true,
						Description: "IP address items to match.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									Required:    true,
									Description: "IP_ADDRESS or SUBNET.",
								},
								"value": schema.StringAttribute{
									Required:    true,
									Description: "IP address or CIDR (e.g. 192.168.1.1, 10.0.0.0/8).",
								},
							},
						},
					},
				},
			},
		},
	}
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
			"logging_enabled":           schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"destination_traffic_filter": trafficFilterSchemaAttr("Destination traffic filter."),
			"source_traffic_filter":      trafficFilterSchemaAttr("Source traffic filter."),
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

// --- Model ↔ API conversion ---

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

	if m.DestinationTrafficFilter != nil {
		req.Destination.TrafficFilter = trafficFilterModelToAPI(m.DestinationTrafficFilter)
	}
	if m.SourceTrafficFilter != nil {
		req.Source.TrafficFilter = trafficFilterModelToAPI(m.SourceTrafficFilter)
	}

	return req
}

func policyAPIToModel(p *network.FirewallPolicyResponse) FirewallPolicyModel {
	model := FirewallPolicyModel{
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

	if p.Destination.TrafficFilter != nil {
		model.DestinationTrafficFilter = trafficFilterAPIToModel(p.Destination.TrafficFilter)
	}
	if p.Source.TrafficFilter != nil {
		model.SourceTrafficFilter = trafficFilterAPIToModel(p.Source.TrafficFilter)
	}

	return model
}

// trafficFilterModelToAPI converts a Terraform traffic filter model to the API map structure.
func trafficFilterModelToAPI(tf *TrafficFilterModel) map[string]any {
	result := map[string]any{
		"type": tf.Type.ValueString(),
	}

	if tf.PortFilter != nil {
		items := make([]map[string]any, len(tf.PortFilter.Items))
		for i, item := range tf.PortFilter.Items {
			items[i] = map[string]any{
				"type":  item.Type.ValueString(),
				"value": int(item.Value.ValueInt64()),
			}
		}
		result["portFilter"] = map[string]any{
			"type":          "PORTS",
			"matchOpposite": boolOrDefault(tf.PortFilter.MatchOpposite, false),
			"items":         items,
		}
	}

	if tf.NetworkFilter != nil {
		var ids []string
		tf.NetworkFilter.NetworkIDs.ElementsAs(context.Background(), &ids, false)
		result["networkFilter"] = map[string]any{
			"networkIds":    ids,
			"matchOpposite": boolOrDefault(tf.NetworkFilter.MatchOpposite, false),
		}
	}

	if tf.IPAddressFilter != nil {
		ipFilter := map[string]any{
			"type":          stringOrDefault(tf.IPAddressFilter.Type, "IP_ADDRESSES"),
			"matchOpposite": boolOrDefault(tf.IPAddressFilter.MatchOpposite, false),
		}
		if !tf.IPAddressFilter.TrafficMatchingListID.IsNull() && !tf.IPAddressFilter.TrafficMatchingListID.IsUnknown() {
			ipFilter["trafficMatchingListId"] = tf.IPAddressFilter.TrafficMatchingListID.ValueString()
		}
		if len(tf.IPAddressFilter.Items) > 0 {
			items := make([]map[string]any, len(tf.IPAddressFilter.Items))
			for i, item := range tf.IPAddressFilter.Items {
				items[i] = map[string]any{
					"type":  item.Type.ValueString(),
					"value": item.Value.ValueString(),
				}
			}
			ipFilter["items"] = items
		}
		result["ipAddressFilter"] = ipFilter
	}

	return result
}

// trafficFilterAPIToModel converts an API traffic filter (map[string]any from JSON) to the Terraform model.
func trafficFilterAPIToModel(raw any) *TrafficFilterModel {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	tf := &TrafficFilterModel{
		Type: stringValue(mapGetString(m, "type")),
	}

	if pf, ok := m["portFilter"].(map[string]any); ok {
		portFilter := &PortFilterModel{
			MatchOpposite: boolValue(mapGetBool(pf, "matchOpposite")),
		}
		if items, ok := pf["items"].([]any); ok {
			for _, rawItem := range items {
				if item, ok := rawItem.(map[string]any); ok {
					portFilter.Items = append(portFilter.Items, PortFilterItemModel{
						Type:  stringValue(mapGetString(item, "type")),
						Value: types.Int64Value(int64(mapGetFloat64(item, "value"))),
					})
				}
			}
		}
		tf.PortFilter = portFilter
	}

	if nf, ok := m["networkFilter"].(map[string]any); ok {
		netFilter := &NetworkFilterModel{
			MatchOpposite: boolValue(mapGetBool(nf, "matchOpposite")),
		}
		if ids, ok := nf["networkIds"].([]any); ok {
			strIDs := make([]string, 0, len(ids))
			for _, raw := range ids {
				if id, ok := raw.(string); ok {
					strIDs = append(strIDs, id)
				}
			}
			listVal, _ := types.ListValueFrom(context.Background(), types.StringType, strIDs)
			netFilter.NetworkIDs = listVal
		}
		tf.NetworkFilter = netFilter
	}

	if ipf, ok := m["ipAddressFilter"].(map[string]any); ok {
		ipFilter := &IPAddressFilterModel{
			Type:          stringValue(mapGetString(ipf, "type")),
			MatchOpposite: boolValue(mapGetBool(ipf, "matchOpposite")),
		}
		if listID, ok := ipf["trafficMatchingListId"].(string); ok && listID != "" {
			ipFilter.TrafficMatchingListID = stringValue(listID)
		} else {
			ipFilter.TrafficMatchingListID = types.StringNull()
		}
		if items, ok := ipf["items"].([]any); ok {
			for _, rawItem := range items {
				if item, ok := rawItem.(map[string]any); ok {
					ipFilter.Items = append(ipFilter.Items, IPAddressItemModel{
						Type:  stringValue(mapGetString(item, "type")),
						Value: stringValue(mapGetString(item, "value")),
					})
				}
			}
		}
		tf.IPAddressFilter = ipFilter
	}

	return tf
}

// JSON map helpers for parsing API responses.

func mapGetString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func mapGetBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func mapGetFloat64(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// expandTrafficFilterNetworkIDs is a helper for extracting network IDs from a types.List.
func expandTrafficFilterNetworkIDs(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	return expandStringList(ctx, list)
}
