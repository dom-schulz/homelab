output "homelab_network_id" {
  description = "NetBird Network ID for the homelab resources."
  value       = netbird_network.homelab.id
}
output "runner_resource_id" {
  description = "NetBird Network Resource ID for the runner VM."
  value       = netbird_network_resource.runner.id
}
