# Controller Firmware Upgrade Guide

This document describes how to handle UniFi controller firmware upgrades when using the OpenTofu/Terraform provider.

## Pre-Upgrade Check

Before upgrading the controller firmware:

```bash
cd systems/network/iac/unifi
tofu plan
```

This establishes a zero-drift baseline. If the plan shows unexpected changes, resolve them before upgrading.

## Post-Upgrade Validation

After the firmware upgrade completes:

### 1. Run a plan to detect API changes

```bash
cd systems/network/iac/unifi
tofu plan
```

**If the plan shows zero changes** — the upgrade is fully compatible. No further action needed.

**If the plan shows errors or unexpected changes** — proceed to step 2.

### 2. Regenerate types from the updated OpenAPI spec

The controller may serve an updated OpenAPI spec with new or changed schemas:

```bash
cd external/go-unifi

# Fetch the latest spec from the controller
curl -sk -H "X-API-KEY: $UNIFI_API_KEY" \
  "$TF_VAR_unifi_base_url/proxy/network/api-docs/integration.json" \
  > openapi/network-integration.json

# Regenerate Go types
go generate ./...

# Review changes
git diff
```

### 3. Run drift detection

```bash
cd external/go-unifi
go run ./cmd/validate/ --base-url "$TF_VAR_unifi_base_url" --api-key "$UNIFI_API_KEY"
```

This compares generated Go struct tags against live API response fields. The report shows:
- **New fields**: Additive changes (safe — `json.Unmarshal` ignores unknown fields)
- **Removed fields**: Breaking changes that may affect resources
- **Type changes**: Fields whose types changed between firmware versions

### 4. Fix and release (if needed)

If the generated types changed:

```bash
cd external/go-unifi
go test ./...           # Ensure SDK tests pass with new types

cd external/terraform-provider-unifi
go test ./...           # Ensure provider tests pass
```

If tests pass, commit and release:

```bash
cd external/go-unifi
git add -A && git commit -m "feat: regenerate types for controller vX.Y.Z"
git tag vX.Y.Z && git push --tags

cd external/terraform-provider-unifi
git add -A && git commit -m "feat: update SDK for controller vX.Y.Z"
git tag vX.Y.Z && git push --tags
```

Then update the module to use the new provider version:

```bash
cd systems/network/iac/unifi
tofu init -upgrade
tofu plan
```

## Forward-Compatibility Guarantees

The provider is designed for forward compatibility:

- **Additive API changes** (new fields, new endpoints): Safe. Go's `json.Unmarshal` ignores unknown fields. No provider changes needed.
- **Removed fields**: Caught by drift detection. May require type regeneration.
- **Changed field types**: Caught by drift detection. May require SDK updates.
- **New resources/endpoints**: Require new resource implementations in the provider.

Most firmware updates require zero provider changes. The drift detection tool catches the cases that do.

## Version Matrix

The provider does not gate on controller version strings. Instead, it uses runtime feature detection:

1. **OpenAPI introspection**: Fetches the spec at configure time, checks for required endpoints
2. **Feature flags**: Queries the controller's feature flag endpoint for enabled capabilities

If a required feature is missing, the provider returns a clear diagnostic message rather than a cryptic API error.
