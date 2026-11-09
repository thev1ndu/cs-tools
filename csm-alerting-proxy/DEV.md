# Dev/test deployment

The cheapest workable stack, using `dev.tfvars`:

| Resource | Setting |
|---|---|
| API Management | `Developer_1` — one unit, no SLA, no zones |
| Service Bus | Standard SKU — no messaging units, 256 KiB message cap, no trusted-services bypass (gateway IPs listed in firewall rules) |
| Ingest policy body cap | 240 KiB (`max_body_bytes` follows the tier automatically) |
| Everything else | unchanged: standard Key Vault (RBAC), 1-GB-then-pay Log Analytics, Consumption Logic Apps, email action group, 7 metric alerts |

Default (prod) behavior is untouched: without `dev.tfvars` the stack is Premium APIM + Premium Service Bus.

## Prerequisites

1. Terraform 1.6+ and `az login` as an identity that can create role assignments (Owner or User Access Administrator + Contributor).
2. A storage account for state. Reuse the production one with a separate key, or create a cheap one for dev:

   ```bash
   az group create -n rg-tfstate-dev -l eastus2
   az storage account create -n sttfstatedev$RANDOM -g rg-tfstate-dev -l eastus2 --sku Standard_LRS
   ```
3. Edit `dev.tfvars` and fill in `subscription_id`, `consumer_principal_ids`, `consumer_ip_ranges`, `operator_ip_ranges`, `alert_email_receivers`, and the current [Cloudflare IPv4 ranges](https://www.cloudflare.com/ips-v4).
4. Create `backend.hcl` (gitignored) pointing at the state account, with a dev key, for example `key = "csm-alerting-proxy/dev.tfstate"`.

## Apply

```bash
cd csm-alerting-proxy
terraform init -backend-config=backend.hcl

terraform validate
terraform plan -var-file=dev.tfvars -out=tfplan.dev
terraform apply tfplan.dev
```

The first apply can fail while the `Key Vault Secrets Officer` assignment propagates; wait a minute and `terraform apply tfplan.dev` again.

Output `gateway_url` for dev is the direct API Management host — no Cloudflare origin is needed for testing:

```bash
echo "terraform output gateway_url"
```

## After apply

```bash
# 1. Set the chat webhook (kept out of Terraform state on purpose).
az keyvault secret set --vault-name "$(terraform output -raw key_vault_name)" \
  --name "$(terraform output -raw chat_webhook_secret_name)" --value '<google chat webhook url>'

# 2. Get the source's provisioning username and password for the smoke test.
KV=$(terraform output -raw key_vault_name)
USER=$(az keyvault secret show --vault-name "$KV" --name alerts-source-aws-username --query value -o tsv)
PASS=$(az keyvault secret show --vault-name "$KV" --name alerts-source-aws --query value -o tsv)

# 3. Send a test alert straight at the gateway (no Cloudflare in front in dev).
curl -i -u "$USER:$PASS" -H 'Content-Type: application/json' \
  -d '{"smoke":"test"}' \
  "$(terraform output -raw gateway_url)/api/wso2/v1/sre_alert_api/aws"
```

Expect `202`, then confirm the message in the `alerts-high` queue in Service Bus Explorer. Keep test bodies under 240 KiB (256 KiB is the Standard-tier hard limit).

## Clean up

Billing is hourly, so destroy when done testing:

```bash
terraform destroy -var-file=dev.tfvars
```

## Cost (eastus2, USD list prices from the Azure retail price API)

| Resource | Meter | Dev monthly (idle, 24/7) |
|---|---|---|
| API Management Developer | $0.0658/hour | ~$48 |
| Service Bus Standard | $10/month base + ~$0.50 per million operations | ~$10 |
| Key Vault standard | $0.03 per 10k operations | < $1 |
| Log Analytics (PerGB2018) | ~$2.30/GB ingested (30-day retention) | ~$2–10, driven by GatewayLogs volume |
| Metric alerts | ~$0.10 per rule, 7 rules | ~$1 |
| Logic Apps (2 workflows) | per-action, Consumption | < $1 |
| **Total running 24/7** | | **~$60–65/month** |

Pure smoke test with destroy right after: about $0.10–0.20 in total (APIM is the meter; ~$0.07/hour times the hours held).

## Not exercised by the dev stack

- Zone redundancy and Premium APIM scale-out (`apim_zones` requires Premium).
- Trusted-services bypass on Service Bus and 1 MiB messages (both Premium-only).
- Cloudflare fronting — dev hits the gateway URL directly, so test the Cloudflare bits in a prod-shaped stack.

Verify these bits on a Premium stack before promoting.
