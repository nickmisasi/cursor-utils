---
name: cursorctl
description: Use `cursorctl`, the Cursor SDK bridge CLI, to run, spawn, and manage Cursor agents from the command line. Trigger when a coding agent needs to delegate work to Cursor cloud sub-agents, run a local Cursor agent, use fire-and-forget delegation, follow up on an existing Cursor agent or run, collect artifacts or conversations, or troubleshoot cursorctl and the Cursor SDK Bridge CLI.
---

# cursorctl

`cursorctl` is a Go CLI over the Cursor SDK Bridge. Its main use is agent-to-agent delegation: a coding agent can start Cursor cloud agents against repositories, detach, inspect their runs later, send follow-ups, and collect results. It also supports local agents that work against this machine's filesystem.

Use explicit cloud flags when delegating. `--repo URL[@ref]` selects cloud; without cloud-only flags the CLI builds local options and uses `--cwd` (default: `--workspace`).

## Start here

```bash
export CURSOR_API_KEY="cursor_..."

# Spawn a cloud agent, wait, and print only the terminal result.
cursorctl agent prompt \
  --repo https://github.com/acme/widgets@main \
  --auto-create-pr \
  --skip-reviewer-request \
  --quiet \
  "Fix the failing tests, verify the fix, and open a PR."
```

The first bridge-backed command downloads the pinned SDK Bridge (`v1.0.27`), verifies its SHA-256 checksum, and caches it under `~/.cache/cursorctl/sdk-bridge/`. Run `cursorctl bridge install` to pre-fetch it.

## The three delegation patterns

### 1. One-shot: `agent prompt`

Create an agent, send one message, consume its run, then close the agent:

```bash
cursorctl agent prompt --repo "$REPO_URL@main" --auto-create-pr --quiet \
  "Implement the issue, run tests, and open a PR."
```

Use for one task with no follow-up. Omit `--quiet` for NDJSON events.

### 2. Fire-and-forget: `agent send --detach`

Create a durable cloud agent, then detach as soon as its run ID is known:

```bash
agent_id="$(cursorctl agent create --repo "$REPO_URL@main" --auto-create-pr |
  jq -r '.agentId')"
cursorctl agent send "$agent_id" --detach "Implement the issue and open a PR."
# {"agentId":"bc-...","runId":"..."} (pretty-printed in default JSON)
```

Save both IDs. Later use `cursorctl run get RUN_ID`, `run wait RUN_ID`, or `run watch RUN_ID`. Detaching closes only the client stream; it does not cancel the server-side run.

### 3. Durable conversation: create, send, follow up

```bash
agent_id="$(cursorctl agent create --repo "$REPO_URL@main" --auto-create-pr |
  jq -r '.agentId')"
cursorctl agent send "$agent_id" --quiet "Implement the issue."
cursorctl agent send "$agent_id" --quiet "Now add regression tests and update the PR."
```

Use when later messages need the same agent's conversation context.

## When to open a reference file

Keep this hub short. Read only the reference needed for the task:

| If the task is... | Read |
| --- | --- |
| Looking up any command, positional argument, flag, RPC, payload, or response | [`references/command-reference.md`](references/command-reference.md) |
| Delegating cloud work, detaching, following up, triaging agents, or collecting results | [`references/delegation-patterns.md`](references/delegation-patterns.md) |
| Parsing streams, choosing JSON/YAML/TOON, using `--json`, or handling exit codes | [`references/output-and-json-mode.md`](references/output-and-json-mode.md) |
| Debugging auth, bridge installation, truncated streams, runtime routing, or failures | [`references/troubleshooting.md`](references/troubleshooting.md) |

Wire-level details are in [`../../docs/sdk-bridge-protocol.md`](../../docs/sdk-bridge-protocol.md).

## Local agents and custom tools

Local agents run on this machine against `--cwd`; a model is required. Cloud agents clone `--repo`; their IDs start with `bc-`.

For local agents, `agent create --custom-tool NAME=COMMAND` declares a tool. Pass the same executor flag to `agent send` so cursorctl runs a loopback callback server. The command receives tool-argument JSON on stdin and should return a JSON object on stdout. See the command reference before using `--custom-tool-config`.

## Top traps

1. Exit `2` is a completed run with terminal status `ERROR`, `CANCELLED`, or `EXPIRED`; exit `1` is a CLI, bridge, RPC, or stream failure. Exit `0` means `FINISHED` for run-returning commands.
2. `--json` is a complete raw RPC request, not an extra options object. It conflicts with positional and per-field flags by default. `--quiet`, `--detach`, artifact `--file`, and custom-tool executor flags are the documented exceptions where applicable.
3. `me`, `models`, and `repos` require the Cursor API key inside `options`; cursorctl injects it, but the key must still resolve from `--api-key` or the environment named by `--api-key-env`.
4. `--detach` is not cancellation. Use `cursorctl run cancel RUN_ID` to request cancellation.
5. `agent send`, `agent prompt`, and `run watch` write one compact JSON event per line regardless of `-o`. Use `--quiet` to suppress events and format only the final result with `-o`.
6. `run watch --after-offset` accepts only an offset previously emitted by `run watch`/`ObserveRun`, not an offset from the initial `agent send` stream.
