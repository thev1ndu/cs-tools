# Feasibility: SRE Alerting Component on Azure

Status: **Planning input.** Assesses whether the solution proposed in the SRE Alert Component architecture review (2026-10-14) can be delivered on Azure, what it would cost, how reliable it can be, and what the alternatives are. It does not define the architecture; that comes after the open questions in section 10 are answered.

Date: 2026-11-05
Input: *SRE Alerting Component, Proposed Architecture* (2026-10-14).
Scope: **Azure only.** The AWS secondary and the Cloudflare multi-cloud failover are noted where they affect an Azure choice but are not costed.

---

## 1. Summary

The proposal is **feasible on Azure** using managed services for every component except the CSM Alert Consumer, which is application code.

| Proposed component | Azure candidates |
|---|---|
| Cloud gateway (pass-through proxy) | API Management (Consumption, v2 or classic tiers), Azure Functions |
| High-availability priority queue | Service Bus (Standard or Premium), two queues |
| Dead-letter queue | Service Bus built-in dead-letter subqueue |
| SRE notification for DLQ | Logic Apps, Azure Monitor alerts |
| CSM database with partitioning and cold storage | Azure Database for PostgreSQL Flexible Server + Blob Storage |

Three points in the proposal **cannot be met exactly as written** and need a decision before the architecture is drawn:

1. **A pass-through gateway cannot place critical alerts on the high-priority queue** without reading the alert, because criticality is inside the alert body (F1).
2. **Sources only learn that an alert was accepted, not that it was processed.** A malformed alert is no longer rejected back to the source; it ends up in the DLQ (F3).
3. **"Moved to cold storage automatically" is not a managed feature** of Azure Database for PostgreSQL; it must be built (F6).

Cost is set almost entirely by **two tier choices**: the gateway tier (about $0 to $5,600 a month) and the queue tier (about $10 to $677 a month). Everything else is under $30 a month at the assumed volume. **API Management Consumption is viable** for this workload under the constraints in section 6.3.

---

## 2. Assumptions

| # | Assumption | Effect if wrong |
|---|---|---|
| A1 | Region `eastus2`, pay-as-you-go list prices in USD, retrieved 2026-11-05 from the Azure Retail Prices API | Prices differ by region and with enterprise agreements; ratios between options hold |
| A2 | 300k alerts a month at baseline, 5M a month as a storm ceiling | Only per-call and per-GB items move, and they stay small (7.3) |
| A3 | Average stored alert about 6 KB (raw payload about 4 KB, structured fields and indexes about 2 KB) | Storage scales linearly; it is not a material cost (7.4) |
| A4 | Largest single alert payload under 1 MiB | Above that, a single queue message cannot hold it (F9) |
| A5 | 730 hours a month | Monthly = hourly price x 730 |
| A6 | CSM Alert Consumer compute and Cloudflare are paid for elsewhere | Not costed here |

---

## 3. Functional requirements

Extracted from the 2026-10-14 proposal.

| ID | Requirement (from the proposal) | Azure feasibility | Notes |
|---|---|---|---|
| FR-01 | Sources post alerts to one public endpoint, `alerts.wso2.com`, over HTTPS | Feasible | Custom domain on the gateway behind Cloudflare |
| FR-02 | Cloudflare routes to Azure (primary) or AWS (secondary) based on health checks | Azure side feasible; AWS out of scope | The Azure gateway must expose a health endpoint Cloudflare can probe. Not built in on every APIM tier (6.3) |
| FR-03 | The gateway authenticates each source | Feasible with constraints | Sources differ in what they can send: Basic credentials, custom headers, signed requests (AWS SNS), Entra tokens (Azure Monitor), challenge-response (GCP sends credentials only after a `401` challenge). See F7 |
| FR-04 | The gateway is a pass-through proxy and does not process alerts | Feasible | Conflicts with FR-07 (F1) |
| FR-05 | Gateway logs are used for request tracing | Feasible, tier dependent | APIM Consumption logs to Application Insights only, not Log Analytics (6.3) |
| FR-06 | Each cloud has a highly available high-priority and normal-priority queue | Feasible | Service Bus has no per-message priority; two queues is the standard pattern. Zone redundancy and SLA depend on tier (6.1) |
| FR-07 | The gateway places critical alerts on the high-priority queue | **Conflict** | Requires reading the alert (F1) |
| FR-08 | The consumer reads the high-priority queue first | Feasible | Strict "high first" can starve the normal queue during a storm (F10) |
| FR-09 | An alert is held on the queue until the consumer acknowledges it | Feasible | Service Bus peek-lock |
| FR-10 | ACK is sent only after the record is committed | Feasible with condition | The database commit must be durable before the ACK (F2) |
| FR-11 | On failure the consumer sends NACK; the message is retried, then moved to the DLQ after a timeout or retry limit | Feasible with condition | Service Bus redelivers immediately after a NACK, so retries can be exhausted in seconds (F4) |
| FR-12 | SRE is notified about messages in the DLQ | Feasible | The notification path must not depend on the alerting pipeline it reports on (F11) |
| FR-13 | One processing service running as replicas, not per-message-type services | Feasible | Competing consumers on the same queues need an idempotency key (F8) |
| FR-14 | The consumer consumes the queues in both clouds | Azure side feasible; AWS out of scope | The same alert may arrive through both clouds during failover (F8) |
| FR-15 | The consumer extracts key fields (for example provider, received time, component) into a structured schema | Feasible | Application logic |
| FR-16 | The raw alert is stored as text on the same record as the structured fields | Feasible with deviation | One payload can hold many alerts (F5) |
| FR-17 | Alerts are retained for at least three years, with no summarisation | Feasible | Storage cost is negligible (7.4) |
| FR-18 | The alerts table is partitioned monthly or quarterly | Feasible | Native PostgreSQL range partitioning |
| FR-19 | Older partitions are compressed and moved to cold storage automatically | Feasible with build | No managed cold tier (F6) |
| FR-20 | Queue length is monitored on both queues | Feasible | Azure Monitor metric alerts on Service Bus |
| FR-21 | All traffic from sources to the gateway uses HTTPS | Feasible | All gateway candidates are HTTPS only |
| FR-22 | Device authentication as an optional additional control | Feasible, limited value | Client certificates work on every APIM tier; few SaaS monitoring tools can present one (F7) |

### 3.1 Missing non-functional requirements

The proposal sets no measurable targets, so no tier choice can be judged right or wrong yet. Proposed for agreement:

| ID | Requirement | Proposed target |
|---|---|---|
| NFR-01 | Intake availability (gateway accepts and the alert is durably queued) | 99.9% a month (43.8 min) |
| NFR-02 | Data loss after the gateway returns 2xx | None |
| NFR-03 | Critical alert latency, receipt to incident | p95 under 60 s when CSM is healthy |
| NFR-04 | DLQ notification latency | Under 2 min |
| NFR-05 | How long alerts wait if the consumer is down | At least 7 days without loss |
| NFR-06 | Monthly run cost | Agreed ceiling |

---

## 4. Findings

| # | Finding | Impact | Needs |
|---|---|---|---|
| F1 | **Priority conflicts with pass-through.** "Critical" is a severity inside the alert body, in a different field and vocabulary per source (Azure Sev0 to Sev4, Datadog 1 to 5, Site24x7 DOWN/CRITICAL/TROUBLE, and so on). A gateway that does not process alerts cannot see it | FR-04 and FR-07 cannot both hold | A decision: route by something the gateway already knows (source, URL), let the gateway read one field, or move prioritisation into the consumer (9.4) |
| F2 | **"ACK after commit" needs a durable commit.** If the database acknowledges a commit before it is on disk, a crash between commit and ACK loses an alert the queue has already deleted | Breaks NFR-02 | Synchronous commit on the write that precedes the ACK |
| F3 | **The source gets an earlier, weaker answer.** The gateway replies when the alert is queued, before it is processed. A malformed or misconfigured alert is accepted and later dead-lettered | Source owners stop seeing their own errors; SRE receives them through the DLQ | Agreement that this is acceptable, and DLQ notifications that say which source and why |
| F4 | **NACK means immediate redelivery.** Service Bus returns an abandoned message at once, so a retry limit of 10 can be used up in seconds during a short database failover | Messages dead-letter during brief outages | Consumer delays before NACK, or reschedules |
| F5 | **One payload can carry many alerts** (Prometheus Alertmanager batches, ServiceNow arrays) | Raw text "on the same record" is duplicated per derived alert, or stored once and referenced | Agreement on FR-16 wording |
| F6 | **No managed cold tier for PostgreSQL on Azure.** Moving old partitions to cold storage is a scheduled export to Blob Storage (Cool, then Archive by lifecycle rule), verify, then drop | A job to build and operate, plus a restore path for audits | Owner for the job |
| F7 | **Gateway authentication cannot be uniform.** AWS SNS signs messages and needs a subscription handshake (the subscriber must fetch a confirmation URL). A pure pass-through gateway can authenticate the request but cannot complete the handshake or verify the signature itself. GCP needs a `401` challenge before it sends credentials | Some source-specific behaviour is needed at the gateway or the consumer | Per-source authentication matrix |
| F8 | **At-least-once delivery means duplicates.** Retries, redeliveries and, later, the same alert arriving through both clouds all produce duplicates | Duplicate records or incidents | A globally unique id stamped at intake and a unique key in the database |
| F9 | **Queue message size is capped.** Service Bus Standard: 256 KiB per message. Premium: 1 MiB by default over HTTP | Larger payloads must be rejected at the gateway | Measured payload size distribution per source |
| F10 | **Strict high-first starves normal alerts** during a storm on the high queue | Normal alerts delayed without bound | Weighted or separate consumption per queue |
| F11 | **The DLQ notifier must be independent** of the gateway, queue consumer and CSM | Otherwise a pipeline failure silences its own alarm | Notification through a separate path (Logic App, Azure Monitor action group) |
| F12 | **Zone redundancy for the gateway exists only in the most expensive APIM tiers** (Premium, Premium v2) | About $5,600 a month for two zones (7.2) | Decision against NFR-01 and the AWS secondary |
| F13 | **No availability, latency or loss targets** in the proposal | Tier choices cannot be justified | Agree NFR-01 to NFR-06 |

---

## 5. Azure service options per component

### 5.1 Gateway

| Option | Pass-through fit | Monthly | SLA | Zones | Notes |
|---|---|---|---|---|---|
| APIM Consumption | Policy-only, no code; can send to Service Bus with managed identity | $0 to $14 | 99.95% | No | Constraints in 6.3 |
| APIM Basic v2 | Same | $150 | 99.95% | No | Positioned by Microsoft for dev/test |
| APIM Standard v2 | Same, plus outbound VNet integration to a private queue | $700 | 99.95% | No | Log Analytics logs, health endpoint |
| APIM Premium v2 | Same, plus VNet injection | $2,800 per unit | 99.99% with zones | Yes | |
| APIM Premium (classic) | Same, plus multi-region | $2,795 per unit | 99.99% with zones | Yes | |
| Azure Functions (Flex Consumption) | Code, not policy; a service to build and maintain | Usage billed, small | 99.95% | Plan dependent | More control over exact bytes and source-specific handshakes |
| Existing shared WSO2 APIM | Same as APIM | Marginal | That instance's | That instance's | Shares blast radius and change windows with other APIs |

### 5.2 Queue

| Option | ACK / NACK | DLQ | Duplicate detection | Max message | Monthly | SLA |
|---|---|---|---|---|---|---|
| Service Bus Premium | Yes (peek-lock) | Yes | Yes | 1 MiB (more over AMQP) | $677 per MU | 99.95% |
| Service Bus Standard | Yes | Yes | Yes | 256 KiB | $10 + operations | 99.9% |
| Service Bus Basic | Yes | Yes | **No** | 256 KiB | Operations only | 99.9% |
| Event Grid namespace (pull) | Yes | To Blob | No | 1 MB | Low | n/a here | 
| Event Hubs | **No per-message ACK** | No | No | 1 MB | n/a | Unsuitable |
| Storage Queues | Visibility timeout | **No** | No | 64 KiB | Low | Unsuitable |

Event Grid namespaces only accept CloudEvents, so the gateway would have to wrap the alert, which conflicts with pass-through.

### 5.3 DLQ notification

| Option | Monthly | Notes |
|---|---|---|
| Logic App (Consumption) reading the DLQ and posting to Google Chat | $5 to $15 | No code; message detail on the card |
| Azure Monitor metric alert on dead-lettered count | About $1 | Count only, no message detail; works as a backstop for the notifier itself |
| Notification sent by the consumer | $0 | Fails exactly when the consumer is the problem |

### 5.4 Database and cold storage

| Option | Notes |
|---|---|
| Azure Database for PostgreSQL Flexible Server | Native partitioning; HA options of 99.9% (none), 99.95% (same zone), 99.99% (zone redundant). No cold tier |
| Blob Storage, Cool / Cold / Archive with lifecycle rules | Target for exported partitions; Archive needs hours to rehydrate |
| Keep everything in PostgreSQL | Simplest; table, vacuum and backup sizes grow for three years |

---

## 6. Reliability

### 6.1 Component SLAs

From the Microsoft SLA for Online Services. Confirm against the current edition before sign-off.

| Service | Option | Monthly SLA |
|---|---|---|
| API Management | Developer | None |
| | Consumption, Basic v2, Standard v2, Basic, Standard | 99.95% |
| | Premium, Premium v2 across availability zones | 99.99% |
| Service Bus | Basic, Standard | 99.9% |
| | Premium | 99.95% |
| Logic Apps | Consumption | 99.9% |
| Azure Database for PostgreSQL Flexible | No HA / same-zone HA / zone-redundant HA | 99.9% / 99.95% / 99.99% |
| Key Vault | Standard | 99.99% |

### 6.2 Composite intake SLA

Intake needs the gateway **and** the queue (in series). Cloudflare is excluded.

| Gateway + queue | Composite | Allowed downtime a month |
|---|---|---|
| APIM Consumption + Service Bus Standard | 99.85% | 65.7 min |
| APIM Consumption + Service Bus Premium | 99.90% | 43.8 min |
| APIM Standard v2 + Service Bus Premium | 99.90% | 43.8 min |
| APIM Premium (zones) + Service Bus Premium | 99.94% | 26.3 min |
| Any Azure pair + AWS secondary through Cloudflare (parallel, independent failures) | about 99.999% | under 1 min |

- APIM Consumption and Standard v2 give the **same** SLA; the $700 difference buys features, not availability.
- Zone-redundant APIM lifts the composite only to 99.94%, because the queue becomes the limit. The proposal's own answer to availability is the **second cloud**, which is the larger step.

### 6.3 APIM Consumption: constraints

From Microsoft Learn (tier feature comparison, gateway overview, gateway runtime limits).

| Constraint | Effect on the proposal |
|---|---|
| No availability zones, VNet or private endpoints | Gateway reaches the queue over its public endpoint with managed identity |
| No static outbound IP | Cannot be allowlisted on a Service Bus Standard IP firewall. Service Bus Premium can admit API Management as a trusted Microsoft service; Standard would stay public with Entra-only access |
| No built-in health endpoint | Cloudflare health checks (FR-02) need a dedicated operation |
| Request logs to Application Insights only, not Log Analytics | Request tracing (FR-05) through Application Insights |
| `rate-limit-by-key` and `quota-by-key` unavailable | Per-source throttling at Cloudflare instead |
| Total request duration capped at 30 s | Gateway-to-queue retries must fit inside 30 s |
| Policy document limited to 16 KiB | Enough for authentication and forwarding; tight for per-source logic |
| Buffered body limit 2 MiB | Above the queue message limit, so not binding |
| Cold start after idle | First request after a quiet period is slower; a frequent health probe keeps it warm |
| Billing: first 1M calls a month free, then $3.50 per million | $0 at baseline, about $14 at the storm ceiling |

**Assessment:** Consumption can meet every functional requirement. Its gaps affect network isolation, zone resilience and log tooling, not whether alerts are delivered. One point should be proven with a short spike before relying on it: Consumption sending to a firewalled Service Bus Premium namespace through the trusted-services setting. Microsoft lists API Management as a trusted service without naming a tier restriction.

### 6.4 Failure scenarios the proposal handles

| Failure | Expected behaviour | Holds if |
|---|---|---|
| Consumer down | Alerts wait on the queue | Message TTL longer than the outage (NFR-05) |
| Database down | NACK, retry, then DLQ | Retry spacing outlasts a database failover (F4) |
| Queue down | Gateway returns 503; sources retry | The source retries; **not every monitoring tool does** (R2) |
| Gateway down (Azure region) | Cloudflare fails over to AWS | AWS secondary exists (out of scope here) |
| Database crash right after ACK | No loss | Commit is durable before ACK (F2) |
| Duplicate delivery | One record | Idempotency key (F8) |
| DLQ notifier down | DLQ grows unnoticed | Independent metric alert (F11) |

---

## 7. Cost

### 7.1 Unit prices (eastus2, list, 2026-11-05)

| Service | Meter | Price | Monthly |
|---|---|---|---|
| APIM Consumption | Calls | First 1M free, then $0.035 per 10K | $3.50 per million over 1M |
| APIM Developer | Unit | $0.0658 / h | $48.03 |
| APIM Basic v2 | Unit, 10M calls included | $0.20548 / h | $150.00 |
| APIM Standard v2 | Unit, 50M calls included | $0.9589 / h | $700.00 |
| APIM Premium (classic) | Unit | $3.829 / h | $2,795.17 |
| APIM Premium v2 | Unit | $3.83562 / h | $2,800.00 |
| Service Bus Basic | Operations | $0.05 per million | about $0 |
| Service Bus Standard | Base, 13M operations included, then $0.80 per million | $10 / month | $10.00 |
| Service Bus Premium | Messaging unit | $0.9275 / h | $677.08 |
| Logic Apps Consumption | Built-in action, first 4,000 free | $0.000025 | $5 to $15 |
| Log Analytics / Application Insights | Ingestion, first 5 GB free | $2.76 / GB | $0 to $15 |
| Key Vault Standard | Operations | $0.03 per 10K | under $1 |
| Azure Monitor | Metric alert time series | $0.10 each | about $1 |
| PostgreSQL Flexible, D2ds v5 (2 vCore) | Compute | $0.178 / h | $129.94 (x2 with HA) |
| PostgreSQL Flexible | Storage | $0.115 / GB-month | 7.4 |
| Blob LRS | Cool / Cold / Archive | $0.01 / $0.0036 / $0.002 per GB-month | 7.4 |

### 7.2 Cost scenarios (Azure intake path, monthly)

| | S0 Lowest | S1 | S2 | S3 Zone redundant |
|---|---|---|---|---|
| Gateway | APIM Consumption, $0 to $14 | APIM Consumption, $0 to $14 | APIM Standard v2, $700 | APIM Premium x2 across zones, $5,590 |
| Queue | Service Bus Standard, $10 | Service Bus Premium 1 MU, $677 | Service Bus Premium 1 MU, $677 | Service Bus Premium 1 MU, $677 |
| DLQ notifier | $10 | $10 | $10 | $10 |
| Logs, secrets, monitoring | $2 to $15 | $2 to $15 | $2 to $15 | $2 to $15 |
| **Monthly** | **about $25 to $50** | **about $690 to $715** | **about $1,390 to $1,400** | **about $6,280 to $6,290** |
| **Yearly** | about $300 to $600 | about $8,300 to $8,600 | about $16,700 | about $75,400 |
| Intake SLA | 99.85% | 99.90% | 99.90% | 99.94% |
| Max alert size | 256 KiB | 1 MiB | 1 MiB | 1 MiB |
| Queue network | Public, Entra only | Firewalled, trusted services | Private endpoint possible | Private endpoint |

Premium v2 with two zones prices within $10 of S3. The database is excluded; see 7.4.

### 7.3 Volume-driven items

| Item | 300k alerts a month | 5M alerts a month |
|---|---|---|
| APIM Consumption calls | $0 | $14 |
| Service Bus Standard operations (about 3 per alert) | $0 | $1.60 |
| Gateway logs, about 2 KB per request, bodies off | $0 | $14 |

### 7.4 Three-year retention

At 300k alerts a month and 6 KB each: about 1.8 GB a month, about 65 GB after three years.

| Layout | Monthly storage cost after 3 years |
|---|---|
| All in PostgreSQL | $7.45 ($14.90 with HA) |
| 6 months in PostgreSQL, rest compressed (about 5:1) in Blob Archive | about $1.27 |

Retention is not a cost driver. Cold storage is justified by table size and by vacuum and backup duration, not by money. A dedicated database server, if needed, adds about $130 a month ($260 with zone-redundant HA).

---

## 8. Risks

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Re-credentialing every monitoring source at the new gateway stalls rollout | High | Medium | Per-source migration with the old path live |
| R2 | A source does not retry on 503, so alerts are lost while the gateway or queue is down | Medium | High | Survey retry behaviour per source |
| R3 | Alerts larger than the queue message limit | Low on Premium, medium on Standard | High | Measure payload sizes per source |
| R4 | APIM Consumption cannot reach a firewalled Service Bus Premium as documented | Low | Medium | Short spike; fall back to Standard v2 |
| R5 | A consumer bug dead-letters everything and floods the SRE space | Medium | Low | Group notifications per source and reason |
| R6 | The cold storage job fails silently and the database grows | Medium | Low | Alert on age of the oldest attached partition |
| R7 | Acknowledged alerts lost on a database crash | Low | High | Durable commit before ACK (F2) |

---

## 9. Alternatives to the proposed approach

### 9.1 Gateway

| Alternative | For | Against |
|---|---|---|
| Managed gateway (APIM), as proposed | No code; authentication, limits and logging are configuration | Policy language is hard to test; source-specific handshakes are awkward |
| Small function (Azure Functions) | Testable code; can handle SNS handshake and exact bytes | Another service to build, patch and deploy |
| No gateway: sources write to the queue directly | Removes a hop | Sources cannot authenticate to Service Bus; not possible for SaaS tools |

### 9.2 Queue

| Alternative | For | Against |
|---|---|---|
| Service Bus, as proposed | ACK/NACK, DLQ, duplicate detection, zones | Premium is the only tier with private networking and 1 MiB messages |
| Database table as the queue | No extra service | Queue and database fail together, which is what the proposal moves away from |
| Event Grid, Event Hubs, Storage Queues | Cheaper | Missing ACK, DLQ or pass-through compatibility (5.2) |

### 9.3 Multi-cloud

| Alternative | For | Against |
|---|---|---|
| Azure + AWS behind Cloudflare, as proposed | Survives a full cloud or region loss; largest availability gain (6.2) | Two gateways, two queues, cross-cloud duplicates, consumer reads from both |
| Azure only, zone redundant | One stack | Region loss stops intake; zone redundancy for the gateway costs about $5,600 a month |
| Azure only, two regions | Same cloud, same tooling | APIM multi-region requires classic Premium; Service Bus geo-replication adds cost |

### 9.4 Priority

| Alternative | Gateway reads the alert | Trigger and resolve on one queue | Notes |
|---|---|---|---|
| Route by source | No | Yes | Keeps pass-through; coarse |
| Route by a URL chosen by the alert owner | No | Not guaranteed | Depends on correct configuration per alert rule |
| Gateway reads one field (for example service) | Yes, one field per source | Yes if both carry the field | Source knowledge at the gateway |
| Gateway reads severity | Yes, per-source mapping | No: a critical trigger and its OK resolve can split | Transform logic at the gateway |
| One queue; the consumer prioritises internally | No | Yes | Extra hop on the critical path |
| No priority, scale consumers on queue depth | No | Yes | Does not meet the agreed decision |

---

## 10. Open questions before architecture

1. **Priority (F1):** what decides "critical" if the gateway does not read the alert?
2. **Targets (F13):** agree NFR-01 to NFR-06, especially the intake SLA and the cost ceiling.
3. **Multi-cloud timing:** is the AWS secondary committed? If yes, Azure zone redundancy for the gateway is hard to justify (6.2).
4. **Source survey:** authentication method, retry behaviour on 503, and largest payload size, per source (F7, R2, R3).
5. **Response contract (F3):** is "accepted, not processed" acceptable to every source owner?
6. **Raw payload with fan-out (F5):** duplicate per alert, or stored once and referenced?
7. **Cold storage ownership (F6):** who builds and runs the export and restore?
8. **Gateway instance:** a dedicated APIM, or the shared WSO2 APIM?

---

## Sources

- Prices: Azure Retail Prices API (`https://prices.azure.com/api/retail/prices`), `armRegionName eq 'eastus2'`, retrieved 2026-11-05.
- APIM tier features, policy support, runtime limits and health endpoint: Microsoft Learn, *Feature-based comparison of Azure API Management tiers*, *API Management gateways overview*, *API Management v2 tiers overview*.
- Service Bus networking and trusted services: Microsoft Learn, *Allow access to Azure Service Bus namespace from specific IP addresses or ranges*.
- SLA percentages: Microsoft *Service Level Agreements for Online Services*; confirm against the current edition.
