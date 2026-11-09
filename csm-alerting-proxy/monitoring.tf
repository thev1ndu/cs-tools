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

# Platform health alerts go to email and the monitor notifier, never through alerts.wso2.com.

locals {
  # Starting thresholds; tune once there is a month of real traffic.
  backlog_threshold = {
    high   = 500
    normal = 5000
  }

  workflows = {
    dlq-notifier     = module.dlq_notifier.id
    monitor-notifier = module.monitor_notifier.id
  }
}

module "platform_action_group" {
  source                    = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Monitor-Action-Groups?ref=v2.41.2"
  montior_action_group_name = "${var.name}-platform"
  short_name                = "csmalerting"
  resource_group_name       = module.resource_group.resource_group_name
  email_receivers = [for r in var.alert_email_receivers : {
    name                    = r.name
    email_address           = r.email_address
    use_common_alert_schema = true
  }]
  actions_webhook_critical = []
  logic_app_receivers = [{
    name                    = "monitor-notifier"
    resource_id             = module.monitor_notifier.id
    callback_url            = module.monitor_notifier_trigger.callback_url
    use_common_alert_schema = true
  }]
  tags = var.tags
}

module "platform_metric_alerts" {
  source              = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Monitor-Metric-Alerts?ref=v2.41.2"
  resource_group_name = module.resource_group.resource_group_name
  location            = var.location
  tags                = var.tags

  metric_alerts_with_2_dimensions = {}

  metric_alerts_with_1_dimension = merge(
    {
      for tier, queue in local.queues : "dead-lettered-${tier}" => {
        alert_name                = "${var.name}-dead-lettered-${tier}"
        description               = "Messages are sitting in the ${queue} dead-letter subqueue"
        scopes                    = [module.servicebus_namespace.id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT1M"
        window_size               = "PT5M"
        severity                  = tier == "high" ? "1" : "2"
        target_resource_type      = "Microsoft.ServiceBus/namespaces"
        criteria_metric_namespace = "Microsoft.ServiceBus/namespaces"
        criteria_metric_name      = "DeadletteredMessages"
        criteria_aggregation      = "Maximum"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = 0
        dimension_1_name          = "EntityName"
        dimension_1_operator      = "Include"
        dimension_1_values        = [queue]
      }
    },
    {
      for tier, queue in local.queues : "backlog-${tier}" => {
        alert_name                = "${var.name}-backlog-${tier}"
        description               = "CSM Alert Core is not keeping up with ${queue}"
        scopes                    = [module.servicebus_namespace.id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT1M"
        window_size               = "PT5M"
        severity                  = tier == "high" ? "1" : "2"
        target_resource_type      = "Microsoft.ServiceBus/namespaces"
        criteria_metric_namespace = "Microsoft.ServiceBus/namespaces"
        criteria_metric_name      = "ActiveMessages"
        criteria_aggregation      = "Minimum"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = local.backlog_threshold[tier]
        dimension_1_name          = "EntityName"
        dimension_1_operator      = "Include"
        dimension_1_values        = [queue]
      }
    },
    {
      gateway-5xx = {
        alert_name                = "${var.name}-gateway-5xx"
        description               = "The alert gateway is answering sources with server errors"
        scopes                    = [module.api_management.id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT1M"
        window_size               = "PT5M"
        severity                  = "1"
        target_resource_type      = "Microsoft.ApiManagement/service"
        criteria_metric_namespace = "Microsoft.ApiManagement/service"
        criteria_metric_name      = "Requests"
        criteria_aggregation      = "Total"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = 5
        dimension_1_name          = "GatewayResponseCodeCategory"
        dimension_1_operator      = "Include"
        dimension_1_values        = ["5xx"]
      }
    },
  )

  metric_alerts = merge(
    {
      servicebus-throttled = {
        alert_name                = "${var.name}-servicebus-throttled"
        description               = "Service Bus is throttling requests; consider more messaging units"
        scopes                    = [module.servicebus_namespace.id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT1M"
        window_size               = "PT5M"
        severity                  = "2"
        target_resource_type      = "Microsoft.ServiceBus/namespaces"
        criteria_metric_namespace = "Microsoft.ServiceBus/namespaces"
        criteria_metric_name      = "ThrottledRequests"
        criteria_aggregation      = "Total"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = 0
      }
      servicebus-server-errors = {
        alert_name                = "${var.name}-servicebus-server-errors"
        description               = "Service Bus is returning server errors"
        scopes                    = [module.servicebus_namespace.id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT1M"
        window_size               = "PT5M"
        severity                  = "1"
        target_resource_type      = "Microsoft.ServiceBus/namespaces"
        criteria_metric_namespace = "Microsoft.ServiceBus/namespaces"
        criteria_metric_name      = "ServerErrors"
        criteria_aggregation      = "Total"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = 0
      }
    },
    {
      for name, id in local.workflows : "${name}-runs-failed" => {
        alert_name                = "${var.name}-${name}-runs-failed"
        description               = "The ${name} Logic App has failed runs, so Chat cards may be missing"
        scopes                    = [id]
        monitor_action_group_id   = module.platform_action_group.monitor_action_group_id
        frequency                 = "PT5M"
        window_size               = "PT15M"
        severity                  = "2"
        target_resource_type      = "Microsoft.Logic/workflows"
        criteria_metric_namespace = "Microsoft.Logic/workflows"
        criteria_metric_name      = "RunsFailed"
        criteria_aggregation      = "Total"
        criteria_operator         = "GreaterThan"
        criteria_threshold        = 0
      }
    },
  )
}

module "workflow_diagnostics" {
  for_each           = local.workflows
  source             = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Resource-Monitoring-Diagnostic-Setting?ref=v2.41.2"
  log_setting_name   = "to-log-analytics"
  target_resource_id = each.value
  archival_locations = {
    archival_storage_account_id    = ""
    log_analytics_workspace_id     = module.log_analytics_workspace.log_analytics_workspace_id
    eventhub_authorization_rule_id = ""
    eventhub_name                  = ""
  }
  required_log_categories = [{ category_name = "WorkflowRuntime" }]
  all_metrics_enabled     = true
}
