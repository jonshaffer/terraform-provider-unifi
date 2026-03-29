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
	_ datasource.DataSource              = &DNSRecordDataSource{}
	_ datasource.DataSourceWithConfigure = &DNSRecordDataSource{}
)

type DNSRecordDataSource struct {
	app *network.App
}

type DNSRecordDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Key        types.String `tfsdk:"key"`
	Value      types.String `tfsdk:"value"`
	RecordType types.String `tfsdk:"record_type"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func NewDNSRecordDataSource() datasource.DataSource {
	return &DNSRecordDataSource{}
}

func (d *DNSRecordDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (d *DNSRecordDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a DNS record by hostname.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true},
			"key":         schema.StringAttribute{Required: true, Description: "Hostname to look up."},
			"value":       schema.StringAttribute{Computed: true},
			"record_type": schema.StringAttribute{Computed: true},
			"enabled":     schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *DNSRecordDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DNSRecordDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DNSRecordDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := d.app.ListDNSRecords(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list DNS records", err.Error())
		return
	}

	key := config.Key.ValueString()
	for _, r := range records {
		if r.Key == key {
			config.ID = stringValue(r.ID)
			config.Value = stringValue(r.Value)
			config.RecordType = stringValue(r.RecordType)
			config.Enabled = boolValue(r.Enabled)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError("DNS record not found", fmt.Sprintf("no record with key %q", key))
}
