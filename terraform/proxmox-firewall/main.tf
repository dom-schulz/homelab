# Host firewall rules for pve itself (host.fw). They only take effect once the
# host firewall is enabled, which is done by hand; see README.md.
#
# Proxmox always accepts 22, 8006, VNC, SPICE and migration ports from its
# local_network (the whole LAN) after these rules. Rules match in order, so the
# last rule drops those ports for everyone not accepted above it. Everything
# else falls to the datacenter input policy (DROP). Replies to pve's own
# connections are accepted before any rule.
resource "proxmox_virtual_environment_firewall_rules" "host" {
  node_name = var.proxmox_node

  rule {
    type    = "in"
    action  = "ACCEPT"
    proto   = "tcp"
    dport   = "22"
    comment = "SSH from anywhere, key-only; the way in when NetBird is down"
  }
  rule {
    type    = "in"
    action  = "ACCEPT"
    iface   = var.netbird_interface
    source  = var.admin_netbird_ip
    proto   = "tcp"
    dport   = "8006"
    comment = "Web UI and API, admin workstation over NetBird"
  }
  rule {
    type    = "in"
    action  = "ACCEPT"
    iface   = var.controller_bridge
    source  = var.controller_ip
    proto   = "tcp"
    dport   = "8006"
    comment = "API, runner controller on its private bridge"
  }
  rule {
    type    = "in"
    action  = "ACCEPT"
    proto   = "udp"
    dport   = tostring(var.netbird_wireguard_port)
    comment = "NetBird WireGuard, direct peer connections"
  }
  rule {
    type    = "in"
    action  = "DROP"
    proto   = "tcp"
    dport   = "8006,3128,5900:5999,60000:60050"
    comment = "Nobody else; overrides the built-in local_network accept"
  }
}
