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
	_ resource.Resource                = &DNSRecordResource{}
	_ resource.ResourceWithImportState = &DNSRecordResource{}
	_ resource.ResourceWithConfigure   = &DNSRecordResource{}
)

type DNSRecordResource struct {
	app *network.App
}

type DNSRecordModel struct {
	ID         types.String `tfsdk:"id"`
	Key        types.String `tfsdk:"key"`
	Value      types.String `tfsdk:"value"`
	RecordType types.String `tfsdk:"record_type"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func NewDNSRecordResource() resource.Resource {
	return &DNSRecordResource{}
}

func (r *DNSRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *DNSRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a static DNS record on the UniFi controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Controller-assigned ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Required:    true,
				Description: "Hostname (e.g., nas.hyperfluid.dev).",
			},
			"value": schema.StringAttribute{
				Required:    true,
				Description: "IP address or target value.",
			},
			"record_type": schema.StringAttribute{
				Required:    true,
				Description: "Record type: A, AAAA, CNAME, MX, NS, TXT, SRV.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the record is active. Defaults to true.",
			},
		},
	}
}

func (r *DNSRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.app.CreateDNSRecord(ctx, network.DNSRecord{
		Key:        plan.Key.ValueString(),
		Value:      plan.Value.ValueString(),
		RecordType: plan.RecordType.ValueString(),
		Enabled:    boolOrDefault(plan.Enabled, true),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())
		return
	}

	plan.ID = stringValue(result.ID)
	plan.Key = stringValue(result.Key)
	plan.Value = stringValue(result.Value)
	plan.RecordType = stringValue(result.RecordType)
	plan.Enabled = boolValue(result.Enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	record, err := r.app.GetDNSRecord(ctx, state.ID.ValueString())
	if err != nil {
		if unifiIsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())
		return
	}

	state.Key = stringValue(record.Key)
	state.Value = stringValue(record.Value)
	state.RecordType = stringValue(record.RecordType)
	state.Enabled = boolValue(record.Enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.app.UpdateDNSRecord(ctx, network.DNSRecord{
		ID:         state.ID.ValueString(),
		Key:        plan.Key.ValueString(),
		Value:      plan.Value.ValueString(),
		RecordType: plan.RecordType.ValueString(),
		Enabled:    boolOrDefault(plan.Enabled, true),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update DNS record", err.Error())
		return
	}

	plan.ID = stringValue(result.ID)
	plan.Key = stringValue(result.Key)
	plan.Value = stringValue(result.Value)
	plan.RecordType = stringValue(result.RecordType)
	plan.Enabled = boolValue(result.Enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.app.DeleteDNSRecord(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
	}
}

func (r *DNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
