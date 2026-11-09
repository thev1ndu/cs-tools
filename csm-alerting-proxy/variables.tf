# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

variable "subscription_id" {
  description = "Azure subscription that hosts the alerting proxy"
  type        = string
}

variable "location" {
  description = "Azure region for every resource in this stack"
  type        = string
}

variable "name" {
  description = "Base name for resources, for example csm-alerting-prod; also feeds the Key Vault name, so keep it short"
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9-]{3,21}$", var.name))
    error_message = "name must be 3 to 21 characters of lowercase letters, digits and hyphens."
  }
}

variable "tags" {
  description = "Tags applied to every taggable resource"
  type        = map(string)
}

variable "sources" {
  description = "Monitoring sources the gateway accepts, one route and credential each"
  type        = set(string)

  validation {
    condition     = alltrue([for s in var.sources : can(regex("^[a-z0-9]+$", s))])
    error_message = "sources must be lowercase alphanumeric."
  }
}

variable "high_tier_services" {
  description = "Services whose alerts go to alerts-high, matched against the service field of the alert body (decision D1); every other alert goes to alerts-normal"
  type        = set(string)

  validation {
    condition     = alltrue([for s in var.high_tier_services : can(regex("^[A-Za-z0-9._-]{1,128}$", s))])
    error_message = "high_tier_services entries must be 1 to 128 letters, digits, dots, underscores or hyphens."
  }
}

variable "apim_sku_name" {
  description = "API Management SKU in <tier>_<units> form, for example Premium_2 (decision D12)"
  type        = string
}

variable "apim_zones" {
  description = "Availability zones for the API Management gateway; Premium only, empty for none"
  type        = list(string)
}

variable "apim_publisher_name" {
  description = "Publisher name shown by API Management"
  type        = string
}

variable "apim_publisher_email" {
  description = "Publisher email for API Management notifications"
  type        = string
}

variable "servicebus_sku" {
  description = "Service Bus tier: Basic (cheapest dev: no IP firewall, no duplicate detection, 14 day TTL), Standard (256 KiB messages, gateway allowed by IP) or Premium (1 MiB messages, gateway allowed as a trusted service)"
  type        = string

  validation {
    condition     = contains(["Basic", "Standard", "Premium"], var.servicebus_sku)
    error_message = "servicebus_sku must be Basic, Standard or Premium."
  }
}

variable "servicebus_capacity" {
  description = "Service Bus Premium messaging units; ignored for Basic and Standard"
  type        = number
}

variable "cloudflare_ip_ranges" {
  description = "Cloudflare IPv4 CIDR ranges (https://www.cloudflare.com/ips-v4); the only callers the gateway accepts. Empty leaves the gateway public (dev only)"
  type        = list(string)

  validation {
    condition     = alltrue([for c in var.cloudflare_ip_ranges : can(cidrhost(c, 0))])
    error_message = "cloudflare_ip_ranges must be a list of IPv4 CIDR ranges."
  }
}

variable "shared_credential" {
  description = "Dev only: one username and password for every source instead of the generated per-source credentials; null keeps per-source credentials"
  type = object({
    username = string
    password = string
  })
  default   = null
  sensitive = true
}

variable "consumer_principal_ids" {
  description = "Entra ID object IDs of the CSM Alert Core identity that receives from the queues"
  type        = list(string)
}

variable "consumer_ip_ranges" {
  description = "Egress IPs or CIDR ranges of CSM Alert Core, allowed through the Service Bus network rules"
  type        = list(string)
}

variable "operator_ip_ranges" {
  description = "IPs or CIDR ranges of operators and CI that manage Key Vault secrets and use Service Bus Explorer"
  type        = list(string)
}

variable "alert_email_receivers" {
  description = "Email receivers for platform health alerts; email is the path that works even when Logic Apps or Chat are down"
  type = list(object({
    name          = string
    email_address = string
  }))
}
