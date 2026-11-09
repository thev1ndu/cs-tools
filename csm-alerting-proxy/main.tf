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

data "azurerm_client_config" "current" {}

module "resource_group" {
  source              = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Resource-Group?ref=v2.41.2"
  resource_group_name = var.name
  location            = var.location
  tags                = var.tags
}

module "log_analytics_workspace" {
  source                       = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Log-Analytics-Workspace?ref=v2.41.2"
  log_analytics_workspace_name = var.name
  location                     = var.location
  resource_group_name          = module.resource_group.resource_group_name
  log_analytics_workspace_sku  = "PerGB2018"
  log_retention_in_days        = 30
  tags                         = var.tags
}

module "key_vault" {
  source                          = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Key-Vault?ref=v2.41.2"
  key_vault_name                  = var.name
  location                        = var.location
  resource_group_name             = module.resource_group.resource_group_name
  sku_name                        = "standard"
  vault_access_tenant_id          = data.azurerm_client_config.current.tenant_id
  enable_vault_rbac_authorization = true
  network_acls_default_action     = "Deny"
  network_acls_bypass             = "AzureServices"
  network_acls_ip_rules           = concat(var.operator_ip_ranges, local.logic_app_outbound_ips)
  tags                            = var.tags
}

# Lets the identity running Terraform write the generated source credentials.
module "operator_key_vault_secrets_officer" {
  source               = "git::https://github.com/wso2/azure-terraform-modules.git//modules/azurerm/Role-Assignment?ref=v2.41.2"
  resource_id          = module.key_vault.vault_id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}
