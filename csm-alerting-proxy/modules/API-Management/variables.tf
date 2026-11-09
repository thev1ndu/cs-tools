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

variable "api_management_abbreviation" {
  description = "Abbreviation for API Manager"
  type        = string
  default     = "apim"
}

variable "api_management_name" {
  description = "Name of the API Manager"
  type        = string
}

variable "location" {
  description = "Location of the API Manager instance"
  type        = string
}

variable "resource_group_name" {
  description = "The name of the resource group in which to create the API Manager"
  type        = string
}

variable "sku_name" {
  description = "The SKU of the API Manager in <tier>_<units> form, for example Premium_2"
  type        = string
}

variable "publisher_name" {
  description = "The name of the publisher"
  type        = string
}

variable "publisher_email" {
  description = "The email of the publisher"
  type        = string
}

variable "min_api_version" {
  description = "The minimum API version"
  type        = string
  default     = "2021-08-01"
}

variable "zones" {
  description = "Availability zones for the gateway; only the Premium tier supports zones. Empty means no zone pinning"
  type        = list(string)
  default     = []
}

variable "identity_type" {
  description = "Managed identity type: SystemAssigned, UserAssigned or SystemAssigned, UserAssigned"
  type        = string
  default     = "SystemAssigned"
}

variable "identity_ids" {
  description = "User assigned identity IDs, required when identity_type includes UserAssigned"
  type        = list(string)
  default     = []
}

variable "tags" {
  description = "Tags for the resource"
  type        = map(string)
}
