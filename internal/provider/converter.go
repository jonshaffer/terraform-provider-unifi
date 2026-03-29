package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stringValue converts a Go string to a Terraform types.String.
func stringValue(s string) types.String {
	return types.StringValue(s)
}

// stringValueOrNull converts a Go string to types.String, returning null for empty strings.
func stringValueOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// boolValue converts a Go bool to a Terraform types.Bool.
func boolValue(b bool) types.Bool {
	return types.BoolValue(b)
}

// int64Value converts a Go int to a Terraform types.Int64.
func int64Value(i int) types.Int64 {
	return types.Int64Value(int64(i))
}

// stringOrDefault returns the value of a types.String or a default if null/unknown.
func stringOrDefault(v types.String, def string) string {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueString()
}

// boolOrDefault returns the value of a types.Bool or a default if null/unknown.
func boolOrDefault(v types.Bool, def bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return v.ValueBool()
}

// int64OrDefault returns the value of a types.Int64 or a default if null/unknown.
func int64OrDefault(v types.Int64, def int) int {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return int(v.ValueInt64())
}
