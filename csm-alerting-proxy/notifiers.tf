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

# Two Logic Apps post to the SRE Google Chat space. Neither is connected to CSM.
# dlq_notifier drains both dead-letter subqueues every minute; monitor_notifier relays Azure Monitor alerts about this platform.

locals {
  servicebus_auth = {
    type     = "ManagedServiceIdentity"
    audience = "https://servicebus.azure.net"
  }

  # Chat card text is HTML, so workflow values are escaped before they are shown.
  html_escape = "replace(replace(replace(%s, '&', '&amp;'), '<', '&lt;'), '>', '&gt;')"

  # Custom properties come back JSON-quoted from the peek, so the quotes are stripped for the card.
  dlq_card_property = {
    for tier in keys(local.queues) : tier => {
      for p in ["source", "service", "received_at", "DeadLetterReason", "DeadLetterErrorDescription"] :
      p => "@{${format(local.html_escape, "replace(coalesce(outputs('peek-${tier}')['headers']?['${p}'], 'unknown'), '\"', '')")}}"
    }
  }

  dlq_drain_actions = {
    for tier, queue in local.queues : tier => {
      type       = "Until"
      expression = "@not(equals(outputs('peek-${tier}')['statusCode'], 201))"
      limit = {
        count   = 20
        timeout = "PT10M"
      }
      actions = {
        "peek-${tier}" = {
          type     = "Http"
          runAfter = {}
          inputs = {
            method         = "POST"
            uri            = "${module.servicebus_namespace.endpoint}/${queue}/$DeadLetterQueue/messages/head?timeout=1"
            authentication = local.servicebus_auth
          }
          runtimeConfiguration = { secureData = { properties = ["outputs"] } }
        }
        "has-message-${tier}" = {
          type       = "If"
          expression = { and = [{ equals = ["@outputs('peek-${tier}')['statusCode']", 201] }] }
          runAfter   = { "peek-${tier}" = ["Succeeded"] }
          actions = {
            "archive-${tier}" = {
              type     = "Http"
              runAfter = {}
              inputs = {
                method = "POST"
                uri    = "${module.servicebus_namespace.endpoint}/${local.dead_letter_archive_queue}/messages"
                headers = {
                  "Content-Type"               = "@{coalesce(outputs('peek-${tier}')['headers']?['Content-Type'], 'application/octet-stream')}"
                  "BrokerProperties"           = "@{string(setProperty(json('{}'), 'MessageId', json(outputs('peek-${tier}')['headers']['BrokerProperties'])['MessageId']))}"
                  "source"                     = "@{coalesce(outputs('peek-${tier}')['headers']?['source'], '\"unknown\"')}"
                  "service"                    = "@{coalesce(outputs('peek-${tier}')['headers']?['service'], '\"unknown\"')}"
                  "tier"                       = "\"${tier}\""
                  "received_at"                = "@{coalesce(outputs('peek-${tier}')['headers']?['received_at'], '\"unknown\"')}"
                  "DeadLetterReason"           = "@{coalesce(outputs('peek-${tier}')['headers']?['DeadLetterReason'], '\"unknown\"')}"
                  "DeadLetterErrorDescription" = "@{coalesce(outputs('peek-${tier}')['headers']?['DeadLetterErrorDescription'], '\"unknown\"')}"
                }
                body           = "@{body('peek-${tier}')}"
                authentication = local.servicebus_auth
              }
              runtimeConfiguration = { secureData = { properties = ["inputs"] } }
            }
            "post-card-${tier}" = {
              type     = "Http"
              runAfter = { "archive-${tier}" = ["Succeeded"] }
              inputs = {
                method  = "POST"
                uri     = "@{body('${module.dlq_notifier_get_webhook.name}')['value']}"
                headers = { "Content-Type" = "application/json; charset=UTF-8" }
                body = {
                  cardsV2 = [{
                    cardId = "dlq-${tier}"
                    card = {
                      header = {
                        title    = "DEAD-LETTERED | ${title(tier)} Priority Alert"
                        subtitle = "#@{json(outputs('peek-${tier}')['headers']['BrokerProperties'])?['MessageId']} | @{replace(coalesce(outputs('peek-${tier}')['headers']?['service'], 'unknown'), '\"', '')} | @{replace(coalesce(outputs('peek-${tier}')['headers']?['source'], 'unknown'), '\"', '')}"
                      }
                      sections = [
                        {
                          widgets = [
                            { textParagraph = { text = "<b>Short Description:</b><br>${local.dlq_card_property[tier]["DeadLetterReason"]}: ${local.dlq_card_property[tier]["DeadLetterErrorDescription"]}" } },
                            { buttonList = { buttons = [{ text = "View Archive", onClick = { openLink = { url = "https://portal.azure.com/#resource${module.dead_letter_archive_queue.id}" } } }] } },
                          ]
                        },
                        {
                          header                    = "Dead-letter Details"
                          collapsible               = true
                          uncollapsibleWidgetsCount = 1
                          widgets = [
                            {
                              textParagraph = {
                                text = join("<br>", [
                                  "<b>Service:</b> ${local.dlq_card_property[tier]["service"]}",
                                  "<b>Source:</b> ${local.dlq_card_property[tier]["source"]}",
                                  "<b>Priority:</b> ${title(tier)}",
                                  "<b>Deliveries:</b> @{json(outputs('peek-${tier}')['headers']['BrokerProperties'])?['DeliveryCount']}",
                                  "<b>Received at gateway:</b> ${local.dlq_card_property[tier]["received_at"]}",
                                  "<b>Enqueued:</b> @{json(outputs('peek-${tier}')['headers']['BrokerProperties'])?['EnqueuedTimeUtc']}",
                                  "<b>Archive:</b> copy kept in ${local.dead_letter_archive_queue} for ${local.dead_letter_archive_days} days",
                                ])
                              }
                            },
                            { textParagraph = { text = "<b>Payload (first 1500 characters):</b><br>@{${format(local.html_escape, "substring(string(body('peek-${tier}')), 0, min(length(string(body('peek-${tier}'))), 1500))")}}" } },
                          ]
                        },
                      ]
                    }
                  }]
                }
              }
              runtimeConfiguration = { secureData = { properties = ["inputs"] } }
            }
            "complete-${tier}" = {
              type     = "Http"
              runAfter = { "post-card-${tier}" = ["Succeeded"] }
              inputs = {
                method         = "DELETE"
                uri            = "@{outputs('peek-${tier}')['headers']['Location']}"
                authentication = local.servicebus_auth
              }
            }
          }
        }
      }
    }
  }
}

module "dlq_notifier" {
  source              = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Workflow?ref=v2.41.2"
  name                = "${var.name}-dlq-notifier"
  location            = var.location
  resource_group_name = data.azurerm_resource_group.this.name
  identity_type       = "SystemAssigned"
}

data "azurerm_logic_app_workflow" "dlq_notifier" {
  name                = module.dlq_notifier.name
  resource_group_name = data.azurerm_resource_group.this.name
  # Waits for the workflow, so the first apply reads it after creating it instead of failing at plan.
  depends_on = [module.dlq_notifier]
}

resource "azurerm_logic_app_trigger_recurrence" "dlq_notifier" {
  name         = "every-minute"
  logic_app_id = module.dlq_notifier.id
  frequency    = "Minute"
  interval     = 1
}

module "dlq_notifier_get_webhook" {
  source         = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Action-HTTP?ref=v2.41.2"
  name           = "get-chat-webhook"
  logic_app_id   = module.dlq_notifier.id
  method         = "GET"
  uri            = "${module.key_vault.vault_uri}secrets/${local.chat_webhook_secret_name}?api-version=7.4"
  authentication = { type = "ManagedServiceIdentity", audience = "https://vault.azure.net" }
  secure_data    = ["outputs"]
}

# High is drained before normal; separate resources because the Logic App rejects a runAfter that names an action not created yet.
resource "azurerm_logic_app_action_custom" "dlq_drain_high" {
  name         = "drain-high"
  logic_app_id = module.dlq_notifier.id
  body = jsonencode(merge(local.dlq_drain_actions["high"], {
    runAfter = { (module.dlq_notifier_get_webhook.name) = ["Succeeded"] }
  }))
}

resource "azurerm_logic_app_action_custom" "dlq_drain_normal" {
  name         = "drain-normal"
  logic_app_id = module.dlq_notifier.id
  body = jsonencode(merge(local.dlq_drain_actions["normal"], {
    runAfter = { (azurerm_logic_app_action_custom.dlq_drain_high.name) = ["Succeeded", "Failed", "Skipped", "TimedOut"] }
  }))
}

module "dlq_notifier_queue_receiver" {
  for_each             = local.queues
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.servicebus_queue[each.key].id
  role_definition_name = "Azure Service Bus Data Receiver"
  principal_id         = data.azurerm_logic_app_workflow.dlq_notifier.identity[0].principal_id
}

module "dlq_notifier_archive_sender" {
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.dead_letter_archive_queue.id
  role_definition_name = "Azure Service Bus Data Sender"
  principal_id         = data.azurerm_logic_app_workflow.dlq_notifier.identity[0].principal_id
}

module "monitor_notifier" {
  source              = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Workflow?ref=v2.41.2"
  name                = "${var.name}-monitor-notifier"
  location            = var.location
  resource_group_name = data.azurerm_resource_group.this.name
  identity_type       = "SystemAssigned"
}

data "azurerm_logic_app_workflow" "monitor_notifier" {
  name                = module.monitor_notifier.name
  resource_group_name = data.azurerm_resource_group.this.name
  # Waits for the workflow, so the first apply reads it after creating it instead of failing at plan.
  depends_on = [module.monitor_notifier]
}

module "monitor_notifier_trigger" {
  source       = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Trigger-HTTP-Request?ref=v2.41.2"
  name         = "azure-monitor-alert"
  logic_app_id = module.monitor_notifier.id
}

module "monitor_notifier_get_webhook" {
  source         = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Action-HTTP?ref=v2.41.2"
  name           = "get-chat-webhook"
  logic_app_id   = module.monitor_notifier.id
  method         = "GET"
  uri            = "${module.key_vault.vault_uri}secrets/${local.chat_webhook_secret_name}?api-version=7.4"
  authentication = { type = "ManagedServiceIdentity", audience = "https://vault.azure.net" }
  secure_data    = ["outputs"]
}

module "monitor_notifier_post_card" {
  source       = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Logic-App-Action-HTTP?ref=v2.41.2"
  name         = "post-card"
  logic_app_id = module.monitor_notifier.id
  method       = "POST"
  uri          = "@{body('${module.monitor_notifier_get_webhook.name}')['value']}"
  headers      = { "Content-Type" = "application/json; charset=UTF-8" }
  body = jsonencode({
    text = join("\n", [
      "*Alerting platform @{triggerBody()?['data']?['essentials']?['monitorCondition']}*: @{triggerBody()?['data']?['essentials']?['alertRule']} (@{triggerBody()?['data']?['essentials']?['severity']})",
      "@{triggerBody()?['data']?['essentials']?['description']}",
      "Resources: @{join(coalesce(triggerBody()?['data']?['essentials']?['alertTargetIDs'], json('[]')), ', ')}",
    ])
  })
  secure_data = ["inputs"]
  run_after   = [{ action_name = module.monitor_notifier_get_webhook.name, action_result = "Succeeded" }]
}

module "notifier_key_vault_secrets_user" {
  for_each = {
    dlq     = data.azurerm_logic_app_workflow.dlq_notifier.identity[0].principal_id
    monitor = data.azurerm_logic_app_workflow.monitor_notifier.identity[0].principal_id
  }
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.key_vault.vault_id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = each.value
}
