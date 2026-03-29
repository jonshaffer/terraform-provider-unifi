resource "unifi_dns_record" "nas" {
  key         = "nas.hyperfluid.dev"
  value       = "192.168.10.39"
  record_type = "A"
  enabled     = true
}
