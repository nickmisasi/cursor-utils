# Cursor SDK Bridge protocol reference

Reference for integrating with [`cursor/sdk-bridge`](https://github.com/cursor/sdk-bridge), pinned to tag **`v1.0.27`** (`@cursor/sdk` 1.0.27). The bridge is a **standalone Bun-packaged binary** (no Node required) that embeds the TypeScript Cursor Agent SDK and exposes it as **Connect RPCs over HTTP/1.1** under protobuf package `sdk.v1`.

```text
┌─────────────────┐  spawn + Connect RPCs   ┌────────────────────┐   HTTPS    ┌─────────────┐
│  your adapter   │ ──────────────────────► │  cursor-sdk-bridge │ ─────────► │  Cursor API │
│ (this Go CLI)   │ ◄────────────────────── │  (local process)   │            │             │
└─────────────────┘  callback RPCs (tools)  └────────────────────┘            └─────────────┘
```

- Wire transport: **HTTP/1.1 only** (classic gRPC over HTTP/2 will NOT work).
- Default bind: `127.0.0.1`, ephemeral port (`--port 0`).
- JSON codec is fully supported (`application/json` unary, `application/connect+json` streaming) — no protobuf codegen required. Field names use proto3 JSON mapping (lowerCamelCase).
- Two auth domains:
  1. **Bridge auth** — per-process bearer token read from `authTokenFile` (from handshake). Send `Authorization: Bearer <token>` on EVERY request, unary and streaming.
  2. **Cursor auth** — `CURSOR_API_KEY` in the bridge process env **and** explicit `apiKey`/`api_key` fields on Create/Resume and every catalog call (catalog has NO env fallback).

## Install

Download from GitHub Releases:

```text
https://github.com/cursor/sdk-bridge/releases/download/v1.0.27/cursor-sdk-bridge-standalone-<os>-<arch>.tar.gz
```

Platforms: `linux|darwin|win32` × `x64|arm64` (win32 x64 only). `SHA256SUMS.txt` published alongside. Archive layout (no top-level dir): `bin/cursor-sdk-bridge`, `proto/sdk/v1/*.proto`, `manifest.json`. Binary path override env: `CURSOR_SDK_BRIDGE_BIN`.

## Spawn / handshake

```bash
CURSOR_API_KEY=... CURSOR_SDK_CLIENT_LANGUAGE=go \
  ./bin/cursor-sdk-bridge --workspace <abs-path>
```

CLI flags: `--host`, `--port`, `--workspace`, `--state-root`, `--local-store`, `--tool-callback-url`, `--tool-callback-auth-token`, `--store-callback-url`, `--store-callback-auth-token`, `--max-concurrent-agents`, `--max-message-bytes`, `--verbose` (or `CURSOR_SDK_BRIDGE_LOG=1`).

Scan **stderr** for a line with prefix exactly `cursor-sdk-bridge ready ` (trailing space), followed by JSON:

```json
{
  "schemaVersion": 1, "serverVersion": "1.0.0", "pid": 3871,
  "transport": "tcp", "protocol": "connect",
  "host": "127.0.0.1", "port": 39217, "url": "http://127.0.0.1:39217",
  "authTokenFile": "/tmp/cursor-sdk-bridge-xxxx/auth-token",
  "workspaceRef": "/tmp", "stateRoot": "~/.cursor/sdk-agent-store/..."
}
```

Rules:
- Validate `schemaVersion == 1`, `transport == "tcp"`, `protocol == "connect"`. Ignore unknown fields.
- ~30s startup timeout.
- Keep draining stderr forever after ready (a full pipe blocks the bridge).
- Never log the raw discovery line (older bridges may inline `authToken`).
- Read `authTokenFile`, trim whitespace → bearer token.

## Wire protocol

**Unary:**

```http
POST http://127.0.0.1:<port>/sdk.v1.<Service>/<Method>
Content-Type: application/json
Authorization: Bearer <token>
Connect-Protocol-Version: 1

{ ...request JSON... }
```

Success: HTTP 200 + JSON response. Error: non-200 + Connect JSON error body.

**Server-streaming** (`Send`, `ObserveRun`, `DownloadArtifact`): request content-type `application/connect+json`. Envelope framing both directions:

```text
[1 byte flags][4 bytes big-endian length][payload]
```

- Flags `0x00` — data frame (JSON message)
- Flags `0x02` — end-of-stream; payload is JSON `{"error":{...}}` only when the RPC failed; `{}` otherwise.

The request body is a single framed message. HTTP status for streams is always 200; stream-level errors live in the EndStream frame.

## Errors

Connect error JSON: `{"code":"...","message":"...","details":[{"type":"sdk.v1.SdkErrorDetails","value":"<unpadded-base64-of-proto>"}]}`. Note the detail value is base64 **protobuf**, not JSON — treat as opaque unless decoding protos.

`SdkErrorDetails`: `request_id?`, `sdk_error_code`, `message`, `help_url?`, `provider?`, `retry_after?`, `rate_limit?`.

`SdkErrorCode` values (proto prefix `SDK_ERROR_CODE_`): `UNAUTHORIZED`, `API_KEY_NOT_FOUND`, `PLAN_REQUIRED`, `ROLE_FORBIDDEN`, `FEATURE_UNAVAILABLE`, `AGENT_NOT_FOUND`, `RUN_NOT_FOUND`, `VALIDATION_ERROR`, `INVALID_MODEL`, `INVALID_BRANCH_NAME`, `REPOSITORY_REQUIRED`, `REPOSITORY_ACCESS`, `PR_RESOLUTION_FAILED`, `USAGE_LIMIT_EXCEEDED`, `AGENT_BUSY`, `AGENT_ARCHIVED`, `RUN_NOT_CANCELLABLE`, `RATE_LIMIT_EXCEEDED`, `UPSTREAM_ERROR`, `INTERNAL_ERROR`, `CLIENT_CANCELLED`.

Bridge auth failure: bare `UNAUTHENTICATED` / `"Unauthorized"` with no details. Run failures are NOT RPC failures — the stream still succeeds with status `ERROR`/`CANCELLED`/`EXPIRED`.

## Streaming events

`Send` / `ObserveRun` yield `RunStreamMessage`:

```protobuf
message RunStreamMessage {
  oneof envelope {
    SdkMessage sdk_message = 1;        // JSON field: sdkMessage
    RunStreamResult result = 2;
    RunStreamDone done = 3;
    InteractionUpdate interaction_update = 5;  // opt-in: SendOptions.enable_deltas
    ConversationStep step = 6;                 // opt-in: SendOptions.enable_steps
  }
  optional string offset = 4;          // opaque exclusive resume token
}
```

- Empty envelope = **keepalive** (~15s idle): ignore, do not advance offset.
- Ignore unknown envelope cases / unknown `SdkMessage.type`.
- Normal end: `result` → `done` → EndStream frame.
- Dropping `Send` does NOT cancel the run — use `ObserveRun` / `WaitLiveRun` / `CancelRun`.
- Resume `ObserveRun` only with offsets from a prior **`ObserveRun`** (not live `Send` offsets).

`SdkMessage.type` values: `system` (subtype `init`), `assistant`, `user`, `tool_call`, `thinking`, `status`, `task`, `usage`, `request`. Payloads are JSON objects (`google.protobuf.Struct`). On run failure, human-readable text often lands in a `status` message even when `RunStreamResult.error_code` is empty.

## RPC catalog

Live capabilities from `GetVersion` (v1.0.27): `agent.create`, `agent.resume`, `agent.send`, `run.observe`, `run.wait`, `run.cancel`, `agent.management`, `cursor.catalog`, `artifacts.chunked`, `agent.usage`.

URL shape: `/sdk.v1.<Service>/<Method>`.

### `SdkAgentService`

| RPC | Streaming | Request | Response |
| --- | --- | --- | --- |
| `CreateAgent` | unary | `{ options: AgentOptions, idempotencyKey? }` | `{ agentId, model: ModelSelection }` |
| `ResumeAgent` | unary | `{ agentId, options: AgentOptions }` | `{ agentId, model }` |
| `ReloadAgent` | unary | `{ agentId }` | `{}` |
| `CloseAgent` | unary | `{ agentId }` | `{}` |
| `Send` | **stream** | `{ agentId, message: UserMessage, options?: SendOptions, idempotencyKey? }` | `stream RunStreamMessage` |
| `WaitLiveRun` | unary | `{ runId }` | `{ result: RunResult }` |
| `GetRun` | unary | `{ runId, options?: GetRunOptions }` | `{ run: RunSnapshot }` |
| `ListRuns` | unary | `{ agentId, options?: ListRunsOptions }` | `{ items: RunSnapshot[], nextCursor }` |
| `GetRunConversation` | unary | `{ runId }` | `{ conversationJson: string }` |
| `ObserveRun` | **stream** | `{ runId, afterOffset? }` | `stream RunStreamMessage` |
| `CancelRun` | unary | `{ runId, agentId? }` | `{}` |
| `GetAgent` | unary | `{ agentId, options?: AgentOperationOptions }` | `{ agent: SdkAgentInfo }` |
| `ListAgents` | unary | `{ options?: ListAgentsOptions }` | `{ items: SdkAgentInfo[], nextCursor }` |
| `ArchiveAgent` | unary | `{ agentId, options? }` | `{}` |
| `UnarchiveAgent` | unary | `{ agentId, options? }` | `{}` |
| `DeleteAgent` | unary | `{ agentId, options? }` | `{}` |
| `ListAgentMessages` | unary | `{ agentId, options?: GetAgentMessagesOptions }` | `{ messages: AgentMessage[] }` |
| `ListArtifacts` | unary | `{ agentId }` | `{ artifacts: SdkArtifact[] }` |
| `DownloadArtifact` | **stream** | `{ agentId, path }` | `stream DownloadArtifactChunk{ data: bytes }` |
| `GetUsage` | unary | `{ agentId, runId? }` | `{ usage: AgentUsage }` (cloud only) |

### `SdkCursorService`

Catalog calls hard-require `options.apiKey` — no env fallback. Missing key → `UNAUTHENTICATED` `"API key is required for cloud catalog calls."`.

| RPC | Request | Response |
| --- | --- | --- |
| `Me` | `{ options: { apiKey } }` | `{ user: SdkUser }` |
| `ListModels` | same | `{ items: SdkModel[] }` |
| `ListRepositories` | same | `{ items: SdkRepository[] }` (`url` only) |

`SdkUser`: `apiKeyName`, `userId`, `userEmail`, `userFirstName`, `userLastName`, `createdAt`.
`SdkModel`: `id`, `displayName`, `description`, `parameters[]`, `variants[]`.

### `SdkBridgeControlService`

| RPC | Request | Response |
| --- | --- | --- |
| `Ping` | `{}` | `{ message }` → `"pong"` |
| `GetVersion` | `{}` | `{ bridgeVersion, protocolVersion, capabilities[] }` |
| `Shutdown` | `{ graceSeconds }` (`0` = immediate) | `{}` |
| `SetToolCallback` | `{ url, authToken }` (empty URL clears) | `{}` |

### Adapter-served callbacks

**`SdkCustomToolCallbackService.CallCustomTool`** — the bridge POSTs to your callback server:
- Req: `{ toolName, args: object, toolCallId?, agentId }`
- Resp: `{ result: object }` — MUST be a JSON object (wrap scalars).
- Register via `--tool-callback-url` + `--tool-callback-auth-token` at launch, or `SetToolCallback` after start. Bridge authenticates **to you** with that token. Local agents only. Declare tools in `LocalAgentOptions.customTools`.

**`SdkStoreCallbackService.CallStore`** — custom local store (launch-time only, `--store-callback-url` + `--local-store '{"type":"custom"}'`):
- Req: `{ substore: "agents"|"runs"|"runEvents"|"checkpoints", method: "get"|"create"|"update"|"delete"|"list"|"append", input: object }`
- Resp: `{ output?: object }` (unset = null). Checkpoint blobs are base64. Callback POSTs may use chunked transfer encoding.

## Key request types (proto3 JSON, lowerCamelCase)

**`AgentOptions`**: `model: ModelSelection` (required for local), `apiKey` (ALWAYS set explicitly), `name`, `local: LocalAgentOptions` XOR `cloud: CloudAgentOptions`, `mcpServers: map<string, McpServerConfig>`, `agents: map<string, AgentDefinition>`, `agentId`, `mode` (`AGENT`|`PLAN` enum in proto; accept `"agent"`/`"plan"` in CLI), `tools` (allowlist, local), `disallowedTools[]`.

**`LocalAgentOptions`**: `cwd[]` (≤1 primary; use `dirs` for multi-root), `settingSources[]`, `sandboxOptions{enabled}`, `store` (`sqlite`|`jsonl`|`custom`), `autoReview?`, `customTools` map, `dirs[]`.

**`CloudAgentOptions`**: `env{type: CLOUD|POOL|MACHINE, name?}`, `repos[]{url, startingRef?, prUrl?}` (1–20), `workOnCurrentBranch?`, `autoCreatePr?`, `skipReviewerRequest?`, `envVars` map (no `CURSOR_*` names), `metadata` map, `openAsCursorGithubApp?`.

**`UserMessage`**: `text`, `images[]` (`url` or base64 `data`+`mimeType`, optional `dimension{width,height}`).

**`SendOptions`**: `model`, `mcpServers`, `local{force?}`, `enableDeltas`, `enableSteps`, `mode`, `cloud{envVars}`.

**`ModelSelection`**: `{ id, params?: [{id, value}] }`.

**`McpServerConfig`**: stdio `{command, args?, env?, cwd?}` (cwd local-only) or http/sse `{url, headers?, auth?{CLIENT_ID, CLIENT_SECRET?, scopes?}}`.

**Option helpers**:

| Message | Fields |
| --- | --- |
| `ListAgentsOptions` | `limit`, `cursor`, `runtime`, `cwd`, `prUrl`, `includeArchived?`, `apiKey` |
| `ListRunsOptions` | `limit`, `cursor`, `runtime`, `cwd`, `apiKey` |
| `GetRunOptions` | `runtime`, `cwd`, `agentId`, `apiKey` |
| `AgentOperationOptions` | `cwd`, `apiKey` |
| `GetAgentMessagesOptions` | `limit`, `offset`, `runtime`, `cwd`, `apiKey` |

## Key response types

- `RunSnapshot` / `RunResult`: `runId`, `agentId`, `status` (`CREATING|RUNNING|FINISHED|ERROR|CANCELLED|EXPIRED`), `result` (assistant text), `model`, `durationMs`, `git`, `createdAt`, `usage?`
- `SdkAgentInfo`: `agentId`, `name`, `summary`, `lastModified`, `status`, `createdAt`, `archived`, `local|cloud` oneof
- `AgentUsage`: totals + `runs[]` of `RunUsage` (`TokenUsage` + optional `UsageCost`)
- `SdkArtifact`: `path`, `sizeBytes`, `updatedAt`
- `AgentMessage`: `type`, `uuid`, `agentId`, `message` (object)
- `TokenUsage`: `inputTokens`, `outputTokens`, `cacheReadTokens`, `cacheWriteTokens`, `totalTokens`, `reasoningTokens?`

## Implementation gotchas

1. Use Connect over HTTP/1.1, not gRPC. JSON codec avoids protobuf codegen entirely.
2. Bearer token on streaming requests too.
3. Keep draining bridge stderr after the ready line.
4. Always set `apiKey` on agent options and catalog RPC options; env-only is insufficient on some builds.
5. Ignore empty stream envelopes (keepalives).
6. Do not feed `Send` offsets into `ObserveRun.afterOffset`.
7. Ignore unknown JSON fields everywhere (including discovery JSON).
8. Tag `v1.0.27`; the `sdk.v1` protocol evolves additively. Pin the bridge archive version.
9. `DownloadArtifact` chunk `data` is proto `bytes` → base64 string in JSON.

## Mapping from TS SDK names

| TS SDK | Bridge RPC |
| --- | --- |
| `Agent.create` / `Agent.resume` | `CreateAgent` / `ResumeAgent` |
| `Agent.prompt` | adapter composite: create → send → wait → close |
| `agent.send` / `run.stream` | `Send` stream (or `ObserveRun`) |
| `run.wait` | `WaitLiveRun` (or drain stream to `result`) |
| `run.cancel` / `run.conversation` | `CancelRun` / `GetRunConversation` |
| `Agent.get`/`list`/`archive`/`unarchive`/`delete` | `GetAgent`/`ListAgents`/`ArchiveAgent`/`UnarchiveAgent`/`DeleteAgent` |
| `Agent.getRun` / `Agent.listRuns` | `GetRun` / `ListRuns` |
| `Agent.messages.list` | `ListAgentMessages` |
| `agent.listArtifacts` / `downloadArtifact` | `ListArtifacts` / `DownloadArtifact` |
| `Agent.getUsage` | `GetUsage` |
| `Cursor.me` / `models.list` / `repositories.list` | `Me` / `ListModels` / `ListRepositories` |
| `agent.reload` / `agent.close` | `ReloadAgent` / `CloseAgent` |
