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

output "gateway_url" {
  description = "API Management gateway URL; the Cloudflare origin for alerts.wso2.com"
  value       = module.api_management.gateway_url
}

output "ingest_urls" {
  description = "Per-source ingest URL on the gateway host; sources use the same path on alerts.wso2.com"
  value       = { for s in keys(var.sources) : s => "${module.api_management.gateway_url}/${local.api_path}/${s}" }
}

output "source_credential_secret_names" {
  description = "Key Vault secrets per source: the password secret and the username secret, so sources are provisioned entirely from the vault"
  value = {
    for s in keys(var.sources) : s => {
      username = "alerts-source-${s}-username"
      password = "alerts-source-${s}"
    }
  }
}

output "key_vault_name" {
  value = module.key_vault.vault_name
}

output "chat_webhook_secret_name" {
  description = "Key Vault secret an operator must set to the Google Chat webhook URL"
  value       = local.chat_webhook_secret_name
}

output "servicebus_namespace" {
  description = "Namespace CSM Alert Core connects to"
  value       = module.servicebus_namespace.name
}

output "queues" {
  description = "Queue per tier, plus the archive queue for dead-lettered messages"
  value       = merge(local.queues, { dead_letter_archive = local.dead_letter_archive_queue })
}
