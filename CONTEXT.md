# cs-tools

WSO2's Customer Success Management (CSM) platform and the integrations that feed it, including the GitHub issue integration.

## GitHub integration

**Service Request**:
The CSM record a validated GitHub issue becomes; one issue produces at most one Service Request.
_Avoid_: SR case, ticket

**Priority**:
How urgent a Service Request is, one of Critical, High, Moderate, Low; the value kept in step with the issue's Priority field.
_Avoid_: Severity (a separate, internal-only S1 to S4 rating)

**Severity**:
A Service Request's internal rating, S1 to S4, set only by WSO2 staff and integration clients and never shown to customers.
_Avoid_: Priority

**Issue Priority field**:
The organisation-level GitHub issue field named Priority (Urgent, High, Medium, Low) shown under an issue's Fields; the GitHub end of Priority.
_Avoid_: Projects priority, priority label, template Priority dropdown
