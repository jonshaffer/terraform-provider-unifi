# terraform-provider-unifi

OpenTofu / Terraform provider for managing UniFi network infrastructure declaratively. Built on [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework) (Protocol 6).

Uses the [go-unifi](https://github.com/jonshaffer/go-unifi) SDK for API communication. See the SDK README for full [API coverage](https://github.com/jonshaffer/go-unifi#api-coverage) across all UniFi resource types.

## Supported Resources

| Resource | Type | Operations | Description |
|----------|------|------------|-------------|
| `unifi_dns_record` | Resource + Data Source | CRUD, Import | Static DNS host records (`A`, `AAAA`, `CNAME`, etc.) |
| `unifi_network` | Resource + Data Source | CRUD, Import | VLAN/network configuration with DHCP and mDNS settings |
| `unifi_firewall_group` | Resource | CRUD, Import | IP address groups and port groups for policy references |
| `unifi_firewall_zone` | Resource + Data Source | CRUD, Import | Zone-based firewall zones with network assignments |
| `unifi_firewall_policy` | Resource | CRUD, Import | Zone-pair firewall policies with traffic filters |
| `unifi_firewall_policy_ordering` | Resource | CRUD | Evaluation order for policies within a zone pair |

**Data sources** support lookup by name (DNS records by `key`, networks by `name`, zones by `name`), returning IDs for cross-resource references.

## Requirements

- UniFi Network controller 10.x+ with API key access
- OpenTofu >= 1.6 or Terraform >= 1.6
- API key authentication (Settings > Control Plane > Integrations)

## Quick Start

Configure the provider:

```hcl
terraform {
  required_providers {
    unifi = {
      source  = "jonshaffer/unifi"
      version = "~> 1.0"
    }
  }
}

provider "unifi" {
  base_url = "https://192.168.1.1"
  site     = "default"
  insecure = true  # self-signed cert
  # api_key read from UNIFI_API_KEY env var
}
```

Manage a DNS record:

```hcl
resource "unifi_dns_record" "nas" {
  key         = "nas.example.com"
  value       = "192.168.10.39"
  record_type = "A"
}
```

Reference system-defined zones in firewall policies:

```hcl
data "unifi_firewall_zone" "gateway" {
  name = "Gateway"
}

resource "unifi_firewall_policy" "allow_dns" {
  name                = "Allow DNS to Gateway"
  action              = "ALLOW"
  source_zone_id      = unifi_firewall_zone.trusted.id
  destination_zone_id = data.unifi_firewall_zone.gateway.id

  destination_traffic_filter {
    type = "PORT"
    port_filter {
      items {
        type  = "PORT_NUMBER"
        value = "53"
      }
    }
  }
}
```

## Authentication

The provider accepts an API key via the `api_key` attribute or `UNIFI_API_KEY` environment variable. Generate an API key in the UniFi controller under Settings > Control Plane > Integrations.

| Attribute | Env Var | Description |
|-----------|---------|-------------|
| `api_key` | `UNIFI_API_KEY` | API key (required) |
| `base_url` | `UNIFI_BASE_URL` | Controller URL (required) |
| `site` | `UNIFI_SITE` | Site name (default: `default`) |
| `insecure` | — | Skip TLS verification (default: `true`) |

## Development

```bash
# Build and install locally
go install .

# Run unit tests
go test ./...

# Run acceptance tests (requires live controller)
TF_ACC=1 go test ./... -v -timeout 30m
```

Configure `~/.tofurc` for local development:

```hcl
provider_installation {
  dev_overrides {
    "jonshaffer/unifi" = "/path/to/go/bin"
  }
  direct {}
}
```
