#!/usr/bin/env bash
# Sends one firing TEST alert per source through the gateway, using the payloads from the alerting API test runbook.
set -euo pipefail

: "${BASE:?set BASE, for example \"$(terraform output -raw gateway_url)/api/wso2/v1/sre_alert_api\"}"
: "${ALERT_AUTH:?set ALERT_AUTH to user:password}"

# Set SERVICE to a service in high_tier_services to send everything to alerts-high.
SERVICE="${SERVICE:-client-test-alert-integration}"
# Alert Core folds repeats of the same alert id into one incident, so every run gets fresh ids.
RUN="${RUN:-$(date +%s)}"
NOW="$(date -u +%Y-%m-%dT%H:%M:%S.000Z)"

send() {
  local source="$1" body="$2"
  printf '%-14s ' "$source"
  curl -sS -w ' -> %{http_code}\n' -X POST "$BASE/$source" -u "$ALERT_AUTH" \
    -H 'Content-Type: application/json' --data-binary "$body"
}

sources="${*:-elasticsearch opensearch azure gcp aws datadog prometheus icinga openobserve site24x7 servicenow}"

for source in $sources; do
  case "$source" in
  elasticsearch) send "$source" '{
  "rule_id": "a8f2c9e1-3b7d-4f6a-9c1e-8d2b5f7a3c9e",
  "rule_name": "TEST ELASTICSEARCH ALERT",
  "trigger_name": "TEST ELASTICSEARCH ALERT",
  "state": "ACTIVE",
  "alert_id": "test-elasticsearch-'"$RUN"'",
  "severity": "1",
  "service": "'"$SERVICE"'",
  "category": "service_interruption",
  "message": "TEST alert from the API test plan. Safe to close."
}' ;;
  opensearch) send "$source" '{
  "monitor_id": "mon-test-1",
  "monitor_name": "TEST OPENSEARCH ALERT",
  "alert_id": "test-opensearch-'"$RUN"'",
  "state": "ACTIVE",
  "severity": "1",
  "envrionment": "production",
  "service": "'"$SERVICE"'",
  "category": "service_interruption",
  "message": "TEST alert from the API test plan. Safe to close."
}' ;;
  azure) send "$source" '{
  "schemaId": "azureMonitorCommonAlertSchema",
  "data": {
    "essentials": {
      "alertId": "test-azure-'"$RUN"'",
      "alertRule": "TEST AZURE ALERT",
      "severity": "Sev0",
      "signalType": "Metric",
      "monitorCondition": "Fired",
      "monitoringService": "Platform",
      "description": "TEST alert from the API test plan. Safe to close."
    },
    "alertContext": {},
    "customProperties": {
      "service": "'"$SERVICE"'",
      "category": "service_interruption",
      "environment": "production"
    }
  }
}' ;;
  gcp) send "$source" '{
  "version": "1.2",
  "incident": {
    "incident_id": "test-gcp-'"$RUN"'",
    "policy_name": "TEST GCP ALERT",
    "condition_name": "TEST GCP CONDITION",
    "state": "open",
    "severity": "critical",
    "summary": "TEST alert from the API test plan. Safe to close.",
    "resource": {"type": "gce_instance", "labels": {}},
    "policy_user_labels": {
      "service": "'"$SERVICE"'",
      "category": "service_interruption",
      "environment": "production"
    }
  }
}' ;;
  aws) send "$source" '{
  "Type": "Notification",
  "MessageId": "test-aws-msg-'"$RUN"'",
  "TopicArn": "arn:aws:sns:us-east-1:123456789012:test-topic",
  "Subject": "TEST AWS ALERT",
  "Message": "{\"AlarmName\":\"TEST AWS ALERT\",\"AlarmDescription\":\"{\\\"service\\\":\\\"'"$SERVICE"'\\\",\\\"severity\\\":\\\"critical\\\",\\\"category\\\":\\\"service_interruption\\\",\\\"environment\\\":\\\"production\\\"}\",\"AWSAccountId\":\"123456789012\",\"NewStateValue\":\"ALARM\",\"NewStateReason\":\"TEST alert from the API test plan. Safe to close.\",\"AlarmArn\":\"test-aws-'"$RUN"'\"}",
  "Timestamp": "'"$NOW"'"
}' ;;
  datadog) send "$source" '{
  "event_id": "9001",
  "event_name": "TEST DATADOG ALERT",
  "alert_id": "test-datadog-'"$RUN"'",
  "transition": "Triggered",
  "body": "TEST alert from the API test plan. Safe to close.",
  "service": "'"$SERVICE"'",
  "severity": "1",
  "category": "service_interruption",
  "environment": "production"
}' ;;
  prometheus) send "$source" '{
  "version": "4",
  "receiver": "sre-webhook",
  "status": "firing",
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "TEST PROMETHEUS ALERT",
      "severity": "critical",
      "service": "'"$SERVICE"'",
      "category": "service_interruption",
      "environment": "production"
    },
    "annotations": {
      "summary": "TEST PROMETHEUS ALERT",
      "description": "TEST alert from the API test plan. Safe to close."
    },
    "startsAt": "'"$NOW"'",
    "fingerprint": "test-prometheus-'"$RUN"'"
  }]
}' ;;
  icinga) send "$source" '{
  "notification_type": "PROBLEM",
  "host_name": "test-icinga-'"$RUN"'",
  "host_display_name": "TEST ICINGA ALERT",
  "host_state": "DOWN",
  "host_output": "TEST alert from the API test plan. Safe to close.",
  "vars": {
    "service": "'"$SERVICE"'",
    "category": "service_interruption",
    "environment": "production"
  }
}' ;;
  openobserve) send "$source" '{
  "short_description": "TEST OPENOBSERVE ALERT",
  "correlation_id": "test-openobserve-'"$RUN"'",
  "description": "TEST alert from the API test plan. Safe to close.",
  "urgency": "1",
  "impact": "1",
  "service": "'"$SERVICE"'",
  "category": "service_interruption",
  "environment": "production"
}' ;;
  site24x7) send "$source" '{
  "MONITORNAME": "TEST SITE24X7 ALERT",
  "MONITOR_ID": "test-site24x7-'"$RUN"'",
  "STATUS": "DOWN",
  "INCIDENT_REASON": "TEST alert from the API test plan. Safe to close.",
  "TAGS": [
    "Service:'"$SERVICE"'",
    "Category:service_interruption",
    "Environment:production"
  ]
}' ;;
  servicenow) send "$source" '{
  "service": "'"$SERVICE"'",
  "metric_name": "TEST SERVICENOW ALERT",
  "severity": "Critical",
  "category": "service_interruption",
  "environment": "production",
  "source": "ServiceNow",
  "unique_identifier": "test-servicenow-'"$RUN"'",
  "description": "TEST alert from the API test plan. Safe to close."
}' ;;
  *) echo "unknown source $source" >&2; exit 1 ;;
  esac
done
