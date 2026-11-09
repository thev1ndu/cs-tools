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

locals {
  wso2_modules = "git::https://github.com/wso2/azure-terraform-modules.git//modules"

  # Same path as the current ingestion service, so sources only change host and credential.
  api_path = "api/wso2/v1/sre_alert_api"

  queues = {
    high   = "alerts-high"
    normal = "alerts-normal"
  }

  # Holds dead-lettered messages after the notifier has posted them, for evidence and manual replay.
  dead_letter_archive_queue = "alerts-dead-letter-archive"

  servicebus_premium = var.servicebus_sku == "Premium"

  # Premium queues accept up to 1 MiB and Standard queues 256 KiB; some headroom is left for
  # broker and custom properties inside the tier's message limit.
  max_body_bytes = local.servicebus_premium ? 983040 : 245760

  # Set by an operator outside Terraform so the webhook URL never lands in state.
  chat_webhook_secret_name = "chat-webhook-sre-alerts"

  cloudflare_address_ranges = [for c in var.cloudflare_ip_ranges : { from = cidrhost(c, 0), to = cidrhost(c, -1) }]

  logic_app_outbound_ips = distinct(concat(
    data.azurerm_logic_app_workflow.dlq_notifier.workflow_outbound_ip_addresses,
    data.azurerm_logic_app_workflow.monitor_notifier.workflow_outbound_ip_addresses,
  ))
}
