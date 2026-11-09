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

resource "azurerm_servicebus_queue" "servicebus_queue" {
  name                                    = var.queue_name
  namespace_id                            = var.namespace_id
  lock_duration                           = var.lock_duration
  max_delivery_count                      = var.max_delivery_count
  default_message_ttl                     = var.default_message_ttl
  dead_lettering_on_message_expiration    = var.dead_lettering_on_message_expiration
  requires_duplicate_detection            = var.requires_duplicate_detection
  duplicate_detection_history_time_window = var.duplicate_detection_history_time_window
  max_size_in_megabytes                   = var.max_size_in_megabytes
  max_message_size_in_kilobytes           = var.max_message_size_in_kilobytes
  partitioning_enabled                    = var.partitioning_enabled
}
