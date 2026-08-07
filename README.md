# cursor-utils

Utilities for building and automating Cursor workflows.

## cursorctl

[`cursorctl`](cursorctl/) is a Go command-line client for the [Cursor SDK Bridge](docs/sdk-bridge-protocol.md). It lets scripts and coding agents create local or cloud Cursor agents, send work, detach from long-running cloud delegation, inspect runs, retrieve conversations and artifacts, and manage agent lifecycle.

Install the latest module:

```bash
go install github.com/nickmisasi/cursor-utils/cursorctl@latest
```

Or build this checkout:

```bash
cd cursorctl
go build -o cursorctl .
```

Set a Cursor API key and delegate one task:

```bash
export CURSOR_API_KEY="cursor_..."

cursorctl agent prompt \
  --repo https://github.com/acme/widgets@main \
  --auto-create-pr \
  --skip-reviewer-request \
  --quiet \
  "Fix the failing tests, verify the change, and open a PR."
```

The pinned standalone SDK Bridge is downloaded and SHA-256 verified on first use. Run `cursorctl bridge install` to pre-fetch it.

For agent-facing, progressive-disclosure guidance, start with the [`cursorctl` skill](skills/cursorctl/SKILL.md). The [`docs/`](docs/) directory contains the bridge wire protocol and TOON output format.

## Repository layout

| Path | Contents |
| --- | --- |
| [`cursorctl/`](cursorctl/) | Go CLI module and CLI-focused README. |
| [`cursorctl/internal/cli/`](cursorctl/internal/cli/) | Command definitions, flag mapping, stream behavior, and exit-code handling. |
| [`cursorctl/internal/bridge/`](cursorctl/internal/bridge/) | Bridge installer, process lifecycle, and Connect RPC client. |
| [`cursorctl/internal/output/`](cursorctl/internal/output/) | JSON, YAML, and TOON output encoders. |
| [`cursorctl/internal/toolserver/`](cursorctl/internal/toolserver/) | Loopback callback executor for local-agent custom tools. |
| [`docs/`](docs/) | SDK Bridge protocol and TOON format references. |
| [`skills/cursorctl/`](skills/cursorctl/) | Agent-facing cursorctl skill and detailed references. |