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

module "api_management" {
  source              = "./modules/API-Management"
  api_management_name = var.name
  location            = var.location
  resource_group_name = data.azurerm_resource_group.this.name
  sku_name            = var.apim_sku_name
  zones               = var.apim_zones
  publisher_name      = var.apim_publisher_name
  publisher_email     = var.apim_publisher_email
  tags                = var.tags
}

# Reads the per-source credentials through named values.
module "apim_key_vault_secrets_user" {
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.key_vault.vault_id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = module.api_management.principal_id
}

# The WSO2 API-Management-API module points its policy at var.api_name instead of the created API name, so the API is declared here.
resource "azurerm_api_management_api" "sre_alert" {
  name                  = "sre-alert-api"
  api_management_name   = module.api_management.name
  resource_group_name   = data.azurerm_resource_group.this.name
  revision              = "1"
  display_name          = "SRE Alert API"
  path                  = local.api_path
  protocols             = ["https"]
  subscription_required = false
}

resource "azurerm_api_management_api_policy" "sre_alert" {
  api_name            = azurerm_api_management_api.sre_alert.name
  api_management_name = module.api_management.name
  resource_group_name = data.azurerm_resource_group.this.name
  xml_content = templatefile("${path.module}/policies/api.xml.tftpl", {
    address_ranges = local.cloudflare_address_ranges
  })
}

module "source_credential" {
  for_each                   = var.sources
  source                     = "git::https://github.com/wso2/azure-terraform-modules.git//modules/random/Random-Password?ref=v2.41.2"
  length                     = 40
  include_special_characters = false
  min_special                = 0
}

# The username is the source name; the secret holds only the password.
module "source_credential_secret" {
  source       = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Key-Vault-Secret?ref=v2.41.2"
  key_vault_id = module.key_vault.vault_id
  secrets_map = {
    for s in var.sources : s => {
      name            = "alerts-source-${s}"
      value           = var.shared_credential == null ? module.source_credential[s].random_password : var.shared_credential.password
      expiration_date = null
    }
  }
  depends_on = [module.operator_key_vault_secrets_officer]
}

resource "azurerm_api_management_named_value" "source_credential" {
  for_each            = var.sources
  name                = "alerts-source-${each.key}"
  api_management_name = module.api_management.name
  resource_group_name = data.azurerm_resource_group.this.name
  display_name        = "alerts-source-${each.key}"
  secret              = true

  value_from_key_vault {
    secret_id = "${module.key_vault.vault_uri}secrets/alerts-source-${each.key}"
  }

  depends_on = [module.source_credential_secret, module.apim_key_vault_secrets_user]
}

module "ingest_operation" {
  for_each            = var.sources
  source              = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/API-Management-API-Operation?ref=v2.41.2"
  api_operation_id    = "ingest-${each.key}"
  api_name            = azurerm_api_management_api.sre_alert.name
  api_management_name = module.api_management.name
  resource_group_name = data.azurerm_resource_group.this.name
  display_name        = "Ingest ${each.key} alert"
  method              = "POST"
  url_template        = "/${each.key}"

  policy_xml_template_file_path = "${path.module}/policies/ingest-operation.xml.tftpl"
  policy_xml_template_vars = {
    source        = each.key
    username      = var.shared_credential == null ? each.key : var.shared_credential.username
    high_services = join(",", sort(var.high_tier_services))
    # The C# sits in an XML attribute, so it is escaped here rather than written escaped in locals.
    service_candidates = replace(replace(replace(replace(lookup(local.service_candidates, each.key, local.default_service_candidates), "&", "&amp;"), "\"", "&quot;"), "<", "&lt;"), ">", "&gt;")
    queue_urls         = { for tier, queue in module.servicebus_queue : tier => "${module.servicebus_namespace.endpoint}/${queue.name}/messages" }
    named_value        = azurerm_api_management_named_value.source_credential[each.key].name
    max_body_bytes     = local.max_body_bytes
  }
}

module "apim_diagnostics" {
  source             = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Resource-Monitoring-Diagnostic-Setting?ref=v2.41.2"
  log_setting_name   = "to-log-analytics"
  target_resource_id = module.api_management.id
  archival_locations = {
    archival_storage_account_id    = ""
    log_analytics_workspace_id     = module.log_analytics_workspace.log_analytics_workspace_id
    eventhub_authorization_rule_id = ""
    eventhub_name                  = ""
  }
  required_log_categories = [{ category_name = "GatewayLogs" }]
  all_metrics_enabled     = true
}
