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
  source                    = "./modules/Service-Bus-Namespace"
  servicebus_namespace_name = var.name
  location                  = var.location
  resource_group_name       = module.resource_group.resource_group_name
  sku                       = var.servicebus_sku
  capacity                  = var.servicebus_capacity
  # On Premium, API Management reaches the namespace as a trusted service, so it needs no IP rule.
  # On Standard there is no trusted-services bypass, so the gateway IPs are allowed explicitly instead.
  network_ip_rules = distinct(concat(var.consumer_ip_ranges, var.operator_ip_ranges, local.logic_app_outbound_ips, local.servicebus_premium ? [] : module.api_management.public_ip_addresses))
  tags             = var.tags
}

module "servicebus_queue" {
  for_each                      = local.queues
  source                        = "./modules/Service-Bus-Queue"
  queue_name                    = each.value
  namespace_id                  = module.servicebus_namespace.id
  max_message_size_in_kilobytes = local.servicebus_premium ? 1024 : null # Large messages are a Premium feature.
}

module "dead_letter_archive_queue" {
  source                               = "./modules/Service-Bus-Queue"
  queue_name                           = local.dead_letter_archive_queue
  namespace_id                         = module.servicebus_namespace.id
  max_message_size_in_kilobytes        = local.servicebus_premium ? 1024 : null # Large messages are a Premium feature.
  default_message_ttl                  = "P30D"
  dead_lettering_on_message_expiration = false
  requires_duplicate_detection         = false
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
  required_log_categories = [{ category_name = "OperationalLogs" }, { category_name = "RuntimeAuditLogs" }]
  all_metrics_enabled     = true
}
