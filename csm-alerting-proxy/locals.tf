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
  # Same path as the current ingestion service, so sources only change host and credential.
  api_path = "api/wso2/v1/sre_alert_api"

  # C# that lists the service names in a parsed source body (`body`); sources not listed carry a top-level service field.
  service_candidates = {
    azure      = "new[] { (string)body.SelectToken(\"data.customProperties.service\") }"
    gcp        = "new[] { (string)body.SelectToken(\"incident.policy_user_labels.service\") }"
    icinga     = "new[] { (string)body.SelectToken(\"vars.service\") }"
    aws        = "new[] { (string)JToken.Parse((string)JToken.Parse((string)body[\"Message\"])[\"AlarmDescription\"])[\"service\"] }"
    prometheus = "body.SelectTokens(\"alerts[*].labels.service\").Select(t => (string)t).ToArray()"
    site24x7   = "body.SelectTokens(\"TAGS[*]\").Select(t => (string)t).Where(t => t != null && t.StartsWith(\"Service:\", StringComparison.OrdinalIgnoreCase)).Select(t => t.Substring(8).Trim()).ToArray()"
  }
  default_service_candidates = "new[] { (string)body.SelectToken(\"service\") }"

  queues = {
    high   = "alerts-high"
    normal = "alerts-normal"
  }

  # Holds dead-lettered messages after the notifier has posted them, for evidence and manual replay.
  dead_letter_archive_queue = "alerts-dead-letter"

  servicebus_premium = var.servicebus_sku == "Premium"
  servicebus_basic   = var.servicebus_sku == "Basic"

  # Basic caps message time to live at 14 days.
  dead_letter_archive_days = local.servicebus_basic ? 14 : 30

  # Basic and Standard have a fixed 256 KiB message limit and reject the setting, so it is left unset there.
  servicebus_max_message_kilobytes = local.servicebus_premium ? 1024 : null

  # Leaves room for broker and custom properties: 960 KiB of the 1 MiB Premium limit, 192 KiB of the 256 KiB Basic and Standard limit.
  max_body_bytes = local.servicebus_premium ? 983040 : 196608

  # Set by an operator outside Terraform so the webhook URL never lands in state.
  chat_webhook_secret_name = "chat-webhook-sre-alerts"

  cloudflare_address_ranges = [for c in var.cloudflare_ip_ranges : { from = cidrhost(c, 0), to = cidrhost(c, -1) }]

  logic_app_outbound_ips = distinct(concat(
    data.azurerm_logic_app_workflow.dlq_notifier.workflow_outbound_ip_addresses,
    data.azurerm_logic_app_workflow.monitor_notifier.workflow_outbound_ip_addresses,
  ))
}
