resource "unifi_network" "servers" {
  name         = "Servers"
  purpose      = "corporate"
  vlan         = 10
  vlan_enabled = true
  ip_subnet    = "192.168.10.1/24"

  dhcpd_enabled = true
  dhcpd_start   = "192.168.10.100"
  dhcpd_stop    = "192.168.10.199"

  mdns_enabled            = false
  internet_access_enabled = true
}
