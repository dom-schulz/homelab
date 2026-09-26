variable "runner_address" {
  description = "Private address of the runner VM exposed through proxmox-host."
  type = string
  default = "<runner-ip>/32"
}
