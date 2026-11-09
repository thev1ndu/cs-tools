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

variable "queue_name" {
  description = "Name of the queue"
  type        = string
}

variable "namespace_id" {
  description = "ID of the Service Bus namespace that holds the queue"
  type        = string
}

variable "lock_duration" {
  description = "Peek-lock duration as an ISO 8601 duration; at most PT5M"
  type        = string
  default     = "PT1M"
}

variable "max_delivery_count" {
  description = "Deliveries before a message is moved to the dead-letter subqueue"
  type        = number
  default     = 10
}

variable "default_message_ttl" {
  description = "Time to live for messages without their own TTL, as an ISO 8601 duration"
  type        = string
  default     = "P7D"
}

variable "dead_lettering_on_message_expiration" {
  description = "Whether expired messages move to the dead-letter subqueue instead of being dropped"
  type        = bool
  default     = true
}

variable "requires_duplicate_detection" {
  description = "Whether the queue drops messages whose MessageId was seen within the detection window; cannot change after creation"
  type        = bool
  default     = true
}

variable "duplicate_detection_history_time_window" {
  description = "Duplicate detection window as an ISO 8601 duration"
  type        = string
  default     = "PT10M"
}

variable "max_size_in_megabytes" {
  description = "Maximum queue size in megabytes"
  type        = number
  default     = 5120
}

variable "max_message_size_in_kilobytes" {
  description = "Maximum message size in kilobytes; Premium only, 1024 to 102400"
  type        = number
  default     = 1024
}

variable "partitioning_enabled" {
  description = "Whether the queue is partitioned"
  type        = bool
  default     = false
}
