# ACP agent integration

Goal: use external ACP agents inside FiveLeaf, add editor and file context to a chat,
and review their changes with the existing Git diff and review comments.

Current evidence: `cmd/fiveleaf/review.go` already formats selections and review notes;
`sidebar.go` provides native views; `internal/plugin/manifest.go` declares agent session
readers, which are separate from live ACP connections. The workspace already contains
the FiveLeaf rename and other changes; preserve them. No `.codegraph` index exists.

## Plan and acceptance

1. [x] Implement ACP v1 stdio transport, process/session lifecycle, updates, permission
   requests and cancellation in `internal/acp`. Verify against a local fake agent,
   including interleaved requests, failures and cancellation. Advertise only supported
   capabilities. Use the official ACP schema and Zed documentation as references.
2. [x] Add user-level `agentServers` settings and declarative plugin `agentServers`,
   with explicit executable/argv/env configuration. Preserve terminal session plugins.
   Verify manifest validation, settings round trips and existing session contributions.
3. [x] Integrate a native Agent sidebar with provider selection, streamed conversation,
   tools, permissions, stop/reconnect and a draft composer. Connect filesystem callbacks
   to editor buffers, protect dirty documents, and close connections with the workspace.
   Verify the UI using the headless harness and a fake ACP subprocess.
4. [x] Connect editor selections, Explorer files, existing context collection and review
   comments to the chat draft. Link tool changes and Git working-tree changes to existing
   diff views. Verify exact selected content, Unicode/ranges, dirty buffers, review
   routing and preservation of pre-existing changes.
5. [x] Document configuration and adapter installation, add translated labels and changelog
   entries, inspect rendered UI states, and run targeted Go/UI checks plus a build.
   Do not run the full test suite. Record the final checks and limitations here.

## Design decisions

- ACP v1 uses newline-delimited JSON-RPC; it is separate from the LSP transport.
- Keep existing terminal session browsing and review sending available. Native chat is
  an explicit destination and inserting context does not send it automatically.
- Agent definitions run only after a user action. Repository folder settings cannot
  define executables. Plugins reuse the existing enable/disable lifecycle.
- Reuse the Git diff/review surfaces; distinguish all working-tree changes from a tool's
  reported diff, since the worktree may already contain the user's edits.
- Filesystem capabilities must expose unsaved text consistently and never silently
  overwrite a dirty buffer. File writes prepare a temporary file in the background;
  the main thread checks the original document's path, lifetime and current edits
  before atomic replacement. A conflict rejects the agent write before queued user
  saves proceed. Unsupported remote ACP connections have an explicit state.

## Progress

- Protocol and UI reference checks completed against ACP stable v1 and Zed external
  agents. One plan critique added plugin-disable handling in background projects,
  plain-text context fallback, and separate worktree/snapshot review.
- `coder/acp-go-sdk v0.13.5` provides the wire types and transport, with no transitive
  dependencies. A wrapper accepts valid empty file writes and bounds message sizes.
- Protocol fake-agent subprocess tests passed with `go test -race ./internal/acp`.
- Settings, plugin manifests and bundled adapters passed the targeted `TestACP`,
  schema/documentation and manifest checks in `internal/settings`, `internal/plugin`
  and `plugins`. Existing terminal-session plugins remain compatible.
- UI end-to-end tests exercise a real fake-agent subprocess: stream, permission,
  editor read/write, immutable diff, inline review, and sending review to a draft.
- The final UI regression run passed:
  `go test -race ./cmd/fiveleaf -run '^TestACP' -count=1` (8.150 seconds).
  The subprocess fixture covers tool notifications supplying `rawInput` and
  `locations` before a permission request containing only `toolCallId`; regression
  checks also cover late, partial and status-only permission-detail updates.
- Review added regression checks for dirty buffers, typing/Save As during a prepared
  write, queued autosave, symlink/hardlink boundaries, stop-timer identity, reconnection
  tool IDs, pending drafts, and snapshot split/reopen/worktree separation.
- Existing Review, AgentContext, plugin-session, sidebar, localization and motion
  tests passed. The full suite was not run, following repository instructions.
- Dark, light/Chinese, permission details and 180px sidebar screenshots rendered and
  inspected; the latest permission-detail and narrow-sidebar checks passed.
- Static checks passed: `go vet ./internal/acp ./cmd/fiveleaf`.
- Build passed: `go build -o /private/tmp/fiveleaf-acp ./cmd/fiveleaf`.
- Formatting and whitespace checks passed: the changed Go files produced no output
  from `gofmt -l`, and the relevant `git diff --check` produced no output.
- All five implementation steps are complete within the boundaries below.

## Current boundaries

This implementation supports local ACP v1 agents and new sessions. It does not yet
restore chat history, provide an ACP registry installer, configure MCP servers, or
advertise client terminal execution or terminal-based login. Existing terminal
sessions and Git workflows remain available. No authenticated Codex/Claude model request was made during
verification; real transport and editor integration were verified with subprocess
fixtures. See `AGENTS-ACP.md` for setup and adapter prerequisites.

The follow-up for diff LSP navigation, native custom registration and additional
bundled agents is tracked in `ACP-FOLLOWUP.md`.
