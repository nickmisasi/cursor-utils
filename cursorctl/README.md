# cursorctl

`cursorctl` is a Go CLI for the Cursor SDK Bridge. It is designed for scripts and coding agents that delegate work to Cursor agents: create a local or cloud agent, send prompts, detach from cloud runs, inspect progress, follow up, and retrieve conversations or artifacts.

The CLI pins SDK Bridge `v1.0.27`. It downloads the standalone binary on first bridge-backed command, verifies the release SHA-256 manifest, and caches it under `~/.cache/cursorctl/sdk-bridge/`.

## Build and install

Install from the module:

```bash
go install github.com/nickmisasi/cursor-utils/cursorctl@latest
```

Build from this directory:

```bash
go build -o cursorctl .
./cursorctl version
```

Representative development-build output:

```json
{
  "bridgeVersion": "v1.0.27",
  "goVersion": "go1.22.2",
  "version": "(devel)"
}
```

Pre-fetch the bridge for later offline use:

```bash
cursorctl bridge install
```

Set the API key before bridge RPC commands:

```bash
export CURSOR_API_KEY="cursor_..."
```

## Global flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--api-key string` | empty | Explicit Cursor API key; overrides `--api-key-env`. |
| `--api-key-env string` | `CURSOR_API_KEY` | Environment variable name containing the key. |
| `--bridge-bin string` | empty | SDK Bridge binary override. |
| `--bridge-version string` | `v1.0.27` | SDK Bridge release to install/use. |
| `--local-store string` | empty | Bridge local-store JSON (`sqlite`/`jsonl`). |
| `-o, --output string` | `json` | Unary/final output: `json`, `yaml`, or `toon`. |
| `--timeout duration` | `0s` | Whole invocation deadline; `0` disables it. |
| `-v, --verbose` | `false` | Show SDK Bridge diagnostics on stderr. |
| `--workspace string` | current directory | Workspace passed to the bridge and default local cwd. |

The explicit key is sensitive; prefer the environment for routine shell use. If `--api-key-env` is changed, missing-key errors name the selected variable.

## Command tree

```text
cursorctl
├── agent
│   ├── create       CreateAgent
│   ├── resume       ResumeAgent
│   ├── send         Send (streaming)
│   ├── prompt       CreateAgent → Send → CloseAgent
│   ├── get          GetAgent
│   ├── list         ListAgents
│   ├── messages     ListAgentMessages
│   ├── usage        GetUsage
│   ├── archive      ArchiveAgent
│   ├── unarchive    UnarchiveAgent
│   ├── delete       DeleteAgent
│   ├── close        CloseAgent
│   └── reload       ReloadAgent
├── run
│   ├── get          GetRun
│   ├── list         ListRuns
│   ├── wait         WaitLiveRun
│   ├── watch        ObserveRun (streaming)
│   ├── cancel       CancelRun
│   └── conversation GetRunConversation
├── artifact
│   ├── list         ListArtifacts
│   └── download     DownloadArtifact (streaming bytes)
├── me               Cursor catalog Me
├── models           Cursor catalog ListModels
├── repos            Cursor catalog ListRepositories
├── bridge
│   ├── install
│   ├── ping
│   └── version
├── completion       bash | fish | powershell | zsh
└── version
```

Run `cursorctl COMMAND --help` for authoritative flag names and defaults. The exhaustive mapping of flags to RPC payloads is in the [skill command reference](../skills/cursorctl/references/command-reference.md).

## Full cloud delegation example

Create a durable cloud agent configured to open a PR:

```bash
repo="https://github.com/acme/widgets@main"

agent_id="$(
  cursorctl agent create \
    --name "fix-tests" \
    --repo "$repo" \
    --auto-create-pr \
    --skip-reviewer-request |
  jq -r '.agentId'
)"
```

Cloud IDs begin with `bc-`. Start work and detach once the run ID is available:

```bash
cursorctl agent send "$agent_id" --detach \
  "Fix the failing tests, run the relevant suite, and open a PR." \
  > detached.json

run_id="$(jq -r '.runId' detached.json)"
```

`--detach` closes observation; it does not cancel the run. Inspect, stream, or wait from a later process:

```bash
cursorctl run get "$run_id" --runtime cloud --agent-id "$agent_id"
cursorctl run watch "$run_id" | tee events.ndjson
cursorctl run wait "$run_id" > result.json

jq -r '.status, .result' result.json
jq -r '.git.branches[]? | .prUrl' result.json
```

Send a follow-up in the same conversation, then inspect outputs:

```bash
cursorctl agent send "$agent_id" --quiet \
  "Add any missing regression tests and update the PR."

cursorctl run conversation "$run_id" > conversation.json
cursorctl artifact list "$agent_id"
cursorctl agent usage "$agent_id"
```

Archive the agent when it no longer needs follow-ups:

```bash
cursorctl agent archive "$agent_id"
```

## Output and process status

Unary commands and `--quiet` results use `-o json|yaml|toon`. Live `agent send`, `agent prompt`, and `run watch` output compact NDJSON regardless of `-o`; each line has an `event` such as `sdkMessage`, `result`, or `done`, plus an optional opaque `offset`.

Exit codes:

- `0`: command succeeded; evaluated run result is `FINISHED`;
- `1`: CLI, bridge, RPC, transport, stream, or formatting failure;
- `2`: evaluated run result is `ERROR`, `CANCELLED`, or `EXPIRED`.

Inspection commands can exit `0` while reporting a failed status because the inspection RPC itself succeeded. A successful detached send also exits `0` before the final run status is known.

Every agent/run/artifact/catalog RPC leaf supports raw `--json` request mode. The value can be a literal object, `@file`, or `-` for stdin; it conflicts with positional and per-field payload flags. Cursorctl injects `apiKey` into request options where required. `agent prompt --json` uses the composite `{options,message,sendOptions,idempotencyKey?}` shape.

## Further reference

- [Agent-facing cursorctl skill](../skills/cursorctl/SKILL.md)
- [Exhaustive command reference](../skills/cursorctl/references/command-reference.md)
- [Cloud delegation recipes](../skills/cursorctl/references/delegation-patterns.md)
- [Output and raw JSON mode](../skills/cursorctl/references/output-and-json-mode.md)
- [Troubleshooting](../skills/cursorctl/references/troubleshooting.md)
- [SDK Bridge wire protocol](../docs/sdk-bridge-protocol.md)
- [TOON format](../docs/toon-format.md)
