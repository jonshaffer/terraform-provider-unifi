package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// int64RequiresReplace returns a plan modifier that requires replace for Int64 attributes.
func int64RequiresReplace() planmodifier.Int64 {
	return &int64RequiresReplaceModifier{}
}

type int64RequiresReplaceModifier struct{}

func (m *int64RequiresReplaceModifier) Description(_ context.Context) string {
	return "If the value changes, Terraform will destroy and recreate the resource."
}

func (m *int64RequiresReplaceModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m *int64RequiresReplaceModifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.PlanValue.Equal(req.StateValue) {
		resp.RequiresReplace = true
	}
}

// boolRequiresReplace returns a plan modifier that requires replace for Bool attributes.
func boolRequiresReplace() planmodifier.Bool {
	return &boolRequiresReplaceModifier{}
}

type boolRequiresReplaceModifier struct{}

func (m *boolRequiresReplaceModifier) Description(_ context.Context) string {
	return "If the value changes, Terraform will destroy and recreate the resource."
}

func (m *boolRequiresReplaceModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m *boolRequiresReplaceModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.PlanValue.Equal(req.StateValue) {
		resp.RequiresReplace = true
	}
}
