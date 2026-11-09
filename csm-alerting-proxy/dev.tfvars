# Cheapest dev/test stack: Developer API Management plus Standard Service Bus, roughly $60 a month.
# Pass with terraform plan/apply -var-file=dev.tfvars and fill in subscription_id and the identifiers below.

subscription_id = "00000000-0000-0000-0000-000000000000"
location        = "eastus2"
name            = "csm-alerting-dev"

tags = {
  product     = "csm"
  component   = "alerting-proxy"
  environment = "dev"
}

# Two routes are enough to exercise both queue tiers.
sources = {
  aws     = { tier = "high" }
  datadog = { tier = "normal" }
}

# D12: Developer_1 is the cheapest API Management tier: one unit, no SLA, no zones. Dev/test only.
apim_sku_name        = "Developer_1"
apim_zones           = []
apim_publisher_name  = "WSO2 SRE"
apim_publisher_email = "sre@example.com"

# Standard is the cheapest Service Bus tier: no messaging units (capacity is ignored), messages
# capped at 256 KiB, and no trusted-services bypass, so the gateway IPs are allowed explicitly.
servicebus_sku      = "Standard"
servicebus_capacity = 1

# Copy from https://www.cloudflare.com/ips-v4 at apply time.
cloudflare_ip_ranges = [
  "173.245.48.0/20",
  "103.21.244.0/22",
]

# Object ID of the identity CSM Alert Core authenticates as.
consumer_principal_ids = ["00000000-0000-0000-0000-000000000000"]
consumer_ip_ranges     = ["203.0.113.0/28"]
operator_ip_ranges     = ["198.51.100.10"]

alert_email_receivers = [
  { name = "sre", email_address = "sre@example.com" },
]
