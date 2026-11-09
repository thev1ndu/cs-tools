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

module "servicebus_namespace" {
  source                       = "./modules/Service-Bus-Namespace"
  servicebus_namespace_name    = var.name
  location                     = var.location
  resource_group_name          = data.azurerm_resource_group.this.name
  sku                          = var.servicebus_sku
  capacity                     = local.servicebus_premium ? var.servicebus_capacity : 0
  premium_messaging_partitions = local.servicebus_premium ? 1 : 0
  # Basic has no IP firewall, so the namespace relies on Entra ID alone; Premium lets API Management in as a trusted service; Standard allows the gateway IPs instead.
  network_rules_enabled            = !local.servicebus_basic
  network_trusted_services_allowed = local.servicebus_premium
  network_ip_rules = distinct(concat(
    var.consumer_ip_ranges,
    var.operator_ip_ranges,
    local.logic_app_outbound_ips,
    local.servicebus_premium ? [] : module.api_management.public_ip_addresses,
  ))
  tags = var.tags
}

module "servicebus_queue" {
  for_each                      = local.queues
  source                        = "./modules/Service-Bus-Queue"
  queue_name                    = each.value
  namespace_id                  = module.servicebus_namespace.id
  max_message_size_in_kilobytes = local.servicebus_max_message_kilobytes
  # Basic has no duplicate detection.
  requires_duplicate_detection = !local.servicebus_basic
}

module "dead_letter_archive_queue" {
  source                               = "./modules/Service-Bus-Queue"
  queue_name                           = local.dead_letter_archive_queue
  namespace_id                         = module.servicebus_namespace.id
  default_message_ttl                  = "P${local.dead_letter_archive_days}D"
  dead_lettering_on_message_expiration = false
  requires_duplicate_detection         = false
  max_message_size_in_kilobytes        = local.servicebus_max_message_kilobytes
}

module "apim_queue_sender" {
  for_each             = local.queues
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.servicebus_queue[each.key].id
  role_definition_name = "Azure Service Bus Data Sender"
  principal_id         = module.api_management.principal_id
}

module "consumer_queue_receiver" {
  for_each = {
    for pair in setproduct(keys(local.queues), var.consumer_principal_ids) : "${pair[0]}-${pair[1]}" => {
      tier         = pair[0]
      principal_id = pair[1]
    }
  }
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.servicebus_queue[each.value.tier].id
  role_definition_name = "Azure Service Bus Data Receiver"
  principal_id         = each.value.principal_id
}

module "servicebus_diagnostics" {
  source             = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Resource-Monitoring-Diagnostic-Setting?ref=v2.41.2"
  log_setting_name   = "to-log-analytics"
  target_resource_id = module.servicebus_namespace.id
  archival_locations = {
    archival_storage_account_id    = ""
    log_analytics_workspace_id     = module.log_analytics_workspace.log_analytics_workspace_id
    eventhub_authorization_rule_id = ""
    eventhub_name                  = ""
  }
  # Runtime audit logs exist only on Premium.
  required_log_categories = concat(
    [{ category_name = "OperationalLogs" }],
    local.servicebus_premium ? [{ category_name = "RuntimeAuditLogs" }] : [],
  )
  all_metrics_enabled = true
}
