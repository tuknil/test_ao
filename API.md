# API Reference

Base URL: `http://localhost:8080` (direct API container: `http://localhost:8081` when running via `docker compose`)

All endpoints accept and return `application/json`. There is no authentication layer — this is a local/demo workspace app.

## Conventions

- **Pagination** (`GET /api/agents`, `GET /api/policies`): `limit` (default `50`, max `200`) and `offset` (default `0`) query params. Responses include `total`, `limit`, `offset` alongside `items`.
- **Search**: a single `search` query param, matched case-insensitively (`ILIKE`) against a set of relevant text columns for that resource.
- **Errors**: non-2xx responses return `{"error": "<message>"}`.

| Status | Meaning |
|---|---|
| `200 OK` | Successful read or update |
| `201 Created` | Resource created |
| `204 No Content` | Successful delete |
| `400 Bad Request` | Invalid body, missing required field, or invalid path param |
| `404 Not Found` | No resource with that id |
| `500 Internal Server Error` | Unexpected server/database error |

---

## Integrations

Generic CRUD for AI Assets Sources / Policy Sinks / Telemetry Sources / Signal Sinks connections (Wiz, Astra, Datalock, ...). `type` is caller-supplied and free-form — the API does not restrict it to a fixed set.

### `GET /api/wiz-integrations`

List all configured integrations.

```bash
curl http://localhost:8080/api/wiz-integrations
```

**Response `200`** — array of:

```json
{
  "id": 1,
  "name": "Wiz - Production",
  "type": "Wiz",
  "baseUrl": "https://api.us1.app.wiz.io",
  "clientId": "prod-client-id",
  "hasSecret": true,
  "mcpServer": "mcp.internal.example.com:8443",
  "createdAt": "2026-08-05T01:38:27.476007-07:00",
  "updatedAt": "2026-08-05T01:38:27.476007-07:00"
}
```

Note: `clientSecret` is **never** returned — only the boolean `hasSecret`.

### `GET /api/wiz-integrations/{id}`

Fetch a single integration by numeric id. `404` if not found.

### `POST /api/wiz-integrations`

Create an integration.

**Request body:**

```json
{
  "name": "Astra - Prod",
  "type": "Astra",
  "baseUrl": "https://api.astra.example.com",
  "clientId": "astra-client-id",
  "clientSecret": "astra-secret-value",
  "mcpServer": ""
}
```

| Field | Required | Notes |
|---|---|---|
| `name` | yes | |
| `type` | no | Defaults to `"Wiz"` if omitted |
| `baseUrl` | yes | |
| `clientId` | yes | |
| `clientSecret` | yes | Write-only |
| `mcpServer` | no | |

**Response `201`** — the created integration (same shape as the list endpoint).

### `PUT /api/wiz-integrations/{id}`

Update an integration. Same body shape as create, except:
- `clientSecret` is **optional** — omit or send `""` to keep the existing secret unchanged.
- `type` is not settable after creation (immutable once created).

**Response `200`** — the updated integration, or `404` if not found.

### `DELETE /api/wiz-integrations/{id}`

**Response `204`**, or `404` if not found.

---

## Agents

Read-only inventory imported from `data/agents.csv` on first boot, plus two mutable fields (`risks` is currently seed-only, `monitor` is toggleable).

### `GET /api/agents`

```bash
curl "http://localhost:8080/api/agents?search=dialogflow&limit=50&offset=0"
```

**Query params:** `search`, `limit`, `offset` (see Conventions). Search matches `name`, `technology_name`, `cloud_platform`, `status`, `region`.

**Response `200`:**

```json
{
  "items": [
    {
      "id": "fb730e4c-798d-5d5f-8daf-2b848657ac2d",
      "agenticOverlayId": "9155e191ad95accc8ca1fcd6705031e4",
      "externalId": "CloudPlatform/VirtualMachine##...",
      "name": "129c16f7",
      "type": "AI_AGENT",
      "nativeType": "hostedAiAgent",
      "technologyName": "Hosted AI Agent",
      "cloudPlatform": "AWS",
      "cloudProvider": "AWS",
      "status": "Active",
      "region": "us-east-1",
      "projects": "Non-production, Astra Production",
      "firstSeen": "2026-05-22T01:42:52Z",
      "createdAt": "",
      "updatedAt": "2026-07-20T09:37:45Z",
      "risks": 0,
      "monitor": false,
      "source": "Wiz-Prod",
      "killSwitchAction": "not taken",
      "riskScore": 0
    }
  ],
  "total": 6772,
  "limit": 50,
  "offset": 0
}
```

`agenticOverlayId` is the MD5 hash of `id`, computed on every request (not stored).

### `PATCH /api/agents/{id}/monitor`

Toggle whether an agent is monitored. `{id}` is the agent's string `id` (not `agenticOverlayId`).

**Request body:**

```json
{ "monitor": true }
```

**Response `200`:**

```json
{ "id": "fb730e4c-798d-5d5f-8daf-2b848657ac2d", "monitor": true }
```

`404` if the id doesn't exist.

### `PATCH /api/agents/{id}/kill-switch-action`

Intended for use by an **external service** (not the workspace UI) to record a kill-switch action taken on an agent.

**Request body:**

```json
{ "action": "deactivated" }
```

`action` must be one of `"not taken"`, `"deactivated"`, `"reactivated"` — any other value returns `400`.

**Response `200`:**

```json
{ "id": "fb730e4c-798d-5d5f-8daf-2b848657ac2d", "killSwitchAction": "deactivated" }
```

`404` if the id doesn't exist.

### `PATCH /api/agents/{id}/risk-score`

Intended for use by an **external service** (not the workspace UI) to push a computed risk score for an agent. Seed data is distributed ~90% at `0`, ~9% in `50`-`60`, ~2% in `70`-`80` — the endpoint itself accepts any integer `0`-`100`.

**Request body:**

```json
{ "riskScore": 82 }
```

`riskScore` must be an integer between `0` and `100` inclusive — any other value returns `400`.

**Response `200`:**

```json
{ "id": "fb730e4c-798d-5d5f-8daf-2b848657ac2d", "riskScore": 82 }
```

`404` if the id doesn't exist.

---

## Policies

Read-only inventory imported from `data/policies.csv` on first boot, plus a toggleable `enabled` field. Each item also includes a **generated, illustrative** Rego snippet — derived from the policy's name/type/severity/cloud fields, not sourced from Wiz or any real policy engine.

### `GET /api/policies`

```bash
curl "http://localhost:8080/api/policies?search=critical&limit=50&offset=0"
```

**Query params:** `search`, `limit`, `offset`. Search matches `name`, `policy_id`, `policy_type`, `update_type`, `severity`, `cloud_platform`.

**Response `200`:**

```json
{
  "items": [
    {
      "id": "4005cc6f-a307-48c9-b8d8-c7eeca4b6b37",
      "policyId": "wc-id-1582",
      "name": "GCP Vertex custom model configured with publicly exposed bucket...",
      "policyType": "CONTROL",
      "updateType": "UPDATE",
      "severity": "CRITICAL",
      "cloudPlatform": "",
      "releasedAt": "2026-07-13T10:41:06Z",
      "applyDate": "2026-07-15T10:46:45Z",
      "regoPolicy": "package wiz.controls.wc_id_1582\n\n# GCP Vertex custom model...\ndeny[msg] {\n\t...\n}\n",
      "enabled": false
    }
  ],
  "total": 450,
  "limit": 50,
  "offset": 0
}
```

### `PATCH /api/policies/{id}/enabled`

**Request body:**

```json
{ "enabled": true }
```

**Response `200`:**

```json
{ "id": "4005cc6f-a307-48c9-b8d8-c7eeca4b6b37", "enabled": true }
```

`404` if the id doesn't exist.

---

## Dashboard

### `GET /api/dashboard/stats`

Live counts computed directly from the `agents` and `policies` tables.

```bash
curl http://localhost:8080/api/dashboard/stats
```

**Response `200`:**

```json
{
  "agentsTotal": 6772,
  "agentsMonitored": 2,
  "policiesTotal": 450,
  "policiesEnabled": 1
}
```
