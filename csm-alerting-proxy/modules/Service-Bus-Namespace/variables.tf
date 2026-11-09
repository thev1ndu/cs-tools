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

variable "servicebus_namespace_abbreviation" {
  description = "Abbreviation for the Service Bus namespace"
  type        = string
  default     = "sbns"
}

variable "servicebus_namespace_name" {
  description = "Name of the Service Bus namespace"
  type        = string
}

variable "location" {
  description = "Location of the Service Bus namespace"
  type        = string
}

variable "resource_group_name" {
  description = "The name of the resource group in which to create the namespace"
  type        = string
}

variable "sku" {
  description = "Service Bus tier; network rules and availability zones need Premium"
  type        = string
  default     = "Premium"
}

variable "capacity" {
  description = "Messaging units for the Premium tier"
  type        = number
  default     = 1
}

variable "premium_messaging_partitions" {
  description = "Partitions for the Premium tier; 1 means not partitioned"
  type        = number
  default     = 1
}

variable "local_auth_enabled" {
  description = "Whether SAS key authentication is allowed; false forces Entra ID"
  type        = bool
  default     = false
}

variable "public_network_access_enabled" {
  description = "Whether the namespace is reachable over the public endpoint (still subject to network rules)"
  type        = bool
  default     = true
}

variable "network_default_action" {
  description = "Action for traffic that matches no network rule: Allow or Deny"
  type        = string
  default     = "Deny"
}

variable "network_trusted_services_allowed" {
  description = "Whether trusted Microsoft services such as API Management bypass the network rules"
  type        = bool
  default     = true
}

variable "network_ip_rules" {
  description = "IP addresses or CIDR ranges allowed through the network rules"
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Tags for the resource"
  type        = map(string)
}
