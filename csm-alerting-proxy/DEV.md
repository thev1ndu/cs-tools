# Dev deployment

The dev stack is cheap and open, for testing the gateway end to end. `terraform.tfvars` (gitignored) holds the real values; `terraform.tfvars.example` shows the same shape.

| Setting | Dev value | Notes |
|---|---|---|
| API Management | `Developer_1`, `apim_zones = []` | ~$48/month, no SLA, no zones |
| Service Bus | `Basic` | No messaging units, no IP firewall, no duplicate detection, 14 day archive |
| Credentials | `shared_credential = { username = "test", password = "test" }` | One login for every source instead of a generated password per source |
| Cloudflare filter | `cloudflare_ip_ranges = []` | Gateway stays public; the shared login is the only gate |
| Body limit | 192 KiB | Basic and Standard cap messages at 256 KiB; Premium allows 960 KiB |
| State | `backend_override.tf` pins `backend "local"` (gitignored) | Delete that file to go back to the azurerm backend |

## Apply

```bash
az login                       # Owner, or User Access Administrator plus Contributor, for the role assignments
cd csm-alerting-proxy

terraform init                 # local backend via backend_override.tf; -backend-config=backend.hcl for the azurerm one
terraform validate
terraform plan -out=tfplan.dev
terraform apply tfplan.dev
```

The first apply can fail writing the source credentials while the `Key Vault Secrets Officer` assignment for your identity propagates; wait a minute and `terraform apply tfplan.dev` again.

The resource group must already exist (it is `data` in `main.tf`): `az group create -n csm-alerting-dev -l eastus2`.

## Test

Everything below is detailed in the README's [Testing](README.md#testing) section; this is the short path.

```bash
GW=$(terraform output -raw gateway_url)     # no Cloudflare in dev; hit the gateway host directly
export BASE="$GW/api/wso2/v1/sre_alert_api"
export ALERT_AUTH='test:test'

# One high and one normal alert
curl -sS -i -X POST "$BASE/elasticsearch" -u "$ALERT_AUTH" -H 'Content-Type: application/json' \
  -d '{"service":"client-prod-alert-integration","rule_name":"TEST HIGH","severity":"1","message":"TEST alert. Safe to close."}'
curl -sS -i -X POST "$BASE/elasticsearch" -u "$ALERT_AUTH" -H 'Content-Type: application/json' \
  -d '{"service":"client-test-alert-integration","rule_name":"TEST NORMAL","severity":"3","message":"TEST alert. Safe to close."}'

# All eleven source shapes from the runbook, fresh alert ids each run
scripts/test-sources.sh
```

Expect `202 {"status":"accepted","request_id":"..."}`. Then set the chat webhook (without it the DLQ card step fails and messages stay in the dead-letter subqueue):

```bash
az keyvault secret set --vault-name "$(terraform output -raw key_vault_name)" \
  --name "$(terraform output -raw chat_webhook_secret_name)" --value '<google chat webhook url>'
```

Dev-specific behaviour to keep in mind:

- **Basic has no duplicate detection**, so a gateway retry can enqueue a duplicate; the README's dead-letter test uses abandonment, which still works.
- **Basic has no IP firewall**: the namespace is reachable from anywhere and Entra ID is the only control.
- **The archive queue holds copies for 14 days** (30 on Premium).

## Clean up

Billing is hourly, so destroy when finished:

```bash
terraform destroy
```

If the Key Vault was soft-deleted from an earlier destroy, purge or recover it first: `az keyvault list-deleted` and `az keyvault purge` (the provider intentionally does not purge on destroy).

## Cost (eastus2, USD list prices from the Azure retail price API)

| Resource | Meter | Dev monthly (idle, 24/7) |
|---|---|---|
| API Management Developer | $0.0658/hour | ~$48 |
| Service Bus Basic | $0.05 per million operations, no base fee | < $1 |
| Key Vault standard | $0.03 per 10k operations | < $1 |
| Log Analytics (PerGB2018) | ~$2.30/GB ingested, 30 day retention | ~$2-10, driven by GatewayLogs volume |
| Metric alerts | ~$0.10 per rule, 9 rules | ~$1 |
| Logic Apps (2 workflows, Consumption) | per action execution | < $1 idle |
| **Total running 24/7** | | **~$50/month** |

A short session (apply, test, destroy) costs about $0.07 of APIM per hour held, plus cents.

Production sizing differs on the two big items: `Premium_2` API Management with two zones is ~$5,590/month and Premium Service Bus is ~$680 per messaging unit, so keep dev on the table above until the cross-cloud and cutover decisions (README decisions D11, D12, D14) are settled.
