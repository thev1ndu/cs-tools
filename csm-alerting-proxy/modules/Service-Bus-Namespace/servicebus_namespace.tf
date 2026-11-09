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

resource "azurerm_servicebus_namespace" "servicebus_namespace" {
  name                          = join("-", [var.servicebus_namespace_abbreviation, var.servicebus_namespace_name])
  location                      = var.location
  resource_group_name           = var.resource_group_name
  sku                           = var.sku
  capacity                      = var.capacity
  premium_messaging_partitions  = var.premium_messaging_partitions
  local_auth_enabled            = var.local_auth_enabled
  public_network_access_enabled = var.public_network_access_enabled
  minimum_tls_version           = "1.2"
  tags                          = var.tags

  dynamic "network_rule_set" {
    for_each = var.network_rules_enabled ? [1] : []
    content {
      default_action                = var.network_default_action
      public_network_access_enabled = var.public_network_access_enabled
      trusted_services_allowed      = var.network_trusted_services_allowed
      ip_rules                      = var.network_ip_rules
    }
  }
}
