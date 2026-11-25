# Multi-agent conversations

Goal: host independent CLI/ACP conversations and an AgentPipe-style roundtable
inside FiveLeaf, with a resizable member list and a separate native MyGo window.

## Plan and acceptance

1. [x] Build a UI-independent `internal/roundtable` scheduler and Adapter interface.
   Verify mentions, capability routing, round-robin fallback, three-hop termination,
   ten-message default context, separate rooms, interruption and stale replies.
2. [x] Implement the CLI adapter using bounded asynchronous subprocess I/O, working
   directory/environment configuration and cancellation of subprocess groups.
   Verify actual helper processes, timeout, error output and prompt transport.
3. [x] Extend agent registration with CLI/ACP type, working directory, tags,
   participation and reply timeout; add JSON import/export. Verify compatibility,
   validation, persistence, conflict handling and no automatic process launch.
4. [x] Preserve independent single chats and connect ACP to the same Adapter
   contract. Reuse editor context, permission choices and immutable diff review.
   Verify switching while a reply/file-context load is pending, room separation,
   workspace teardown and existing ACP protections.
5. [x] Add a collapsible, resizable roster and a separate MyGo roundtable window.
   Verify timeline attribution, member status, stop/clear/system/context controls,
   window lifecycle, menu routing and light/dark/narrow layouts.
6. [x] Document usage and boundaries, translate labels, update the changelog and
   run the focused regression checks, vet and build. Do not run the full suite.

## Design decisions and critique

- ACP definitions keep their existing default protocol; CLI is explicit. Commands
  remain executable plus argument array, without shell parsing.
- Capability tags choose a speaker; they do not grant permissions. Every enabled
  member shares the group transcript when selected, and one member speaks at a time.
- A direct-message room produces one reply. A roundtable reply is routed again,
  with an enforced three-hop ceiling. A new human message interrupts the old chain.
- Each room owns its transcript and system prompt. ACP roundtable calls use their
  own sessions so direct chats cannot leak into group context.
- Existing ACP editor callbacks require stable chat identity, not the selected
  sidebar state. Late context loads and write results must retain their recipient.
- A separate window must not replace the IDE's UI services or send menu commands
  to an editor behind it. Closing a workspace cancels all of its agent work.
- Configuration import merges validated entries and reports ID conflicts. Export
  files use private permissions because environment values may contain credentials.
- Optional LLM arbitration, token budgets and SQLite transcript storage are left
  as extension points. JSON configuration, deterministic scheduling and in-memory
  transcripts cover the required core without adding a database dependency.

## Ownership

- Core worker: `internal/roundtable` scheduler, CLI adapter and tests.
- Configuration worker: settings/plugin schema, registration and import/export UI.
- UI worker: roster, conversation layout, roundtable window and its menu handling.
- Root: stable chat ownership, ACP adapter, lifecycle integration, locales and docs.

## Verification

- `go test -race ./internal/roundtable -count=1` passed (3.665s). Real helper
  processes cover stdin/final-argument prompts, working directories, environment,
  stderr, UTF-8, output bounds, timeouts and Unix process-group cleanup, including
  descendants of a parent that already exited. Windows amd64 cross-compilation
  passed; Windows process cleanup was not exercised on a Windows machine.
- Scheduler regressions cover leading mentions, unknown and disabled targets,
  longest-name matching, capability routing, round-robin, bounded context, three
  hops, interruption and stale replies. Invalid new targets preserve active calls.
- Settings and plugin configuration checks passed, including strict JSON imports,
  compatibility defaults, all-or-nothing conflicts and private export files.
  Native registration/import/export tests passed (5.603s), and their race run
  passed (43.988s). Three translated registration screenshots were inspected.
- ACP lifecycle regressions use an independent subprocess fixture. Stopping a
  pending second session, reapplying modes, rejecting obsolete-session requests
  and rejecting a prepared write after a session change passed normally (1.332s)
  and under the race detector (5.878s).
- Roster and roundtable UI checks passed normally and under the race detector.
  English dark, Chinese light and Japanese dark screenshots were inspected,
  including a 640px roundtable. The splitter and transcript stretch with the
  window, and background chats retain their draft, scroll and permission state.
- Independent review found and prompted fixes for unavailable directed targets,
  immediate global disable propagation to background projects, and native edit
  menu actions in the roundtable. All three fixes were rechecked by the reviewer.
  Target and native-menu regressions passed in the UI group (5.358s) and its race
  run (9.037s). They cover disabled and removed targets, explicit recovery and
  native undo/redo/cut/select-all without changing the background editor.
- Global configuration regressions run real CLI helpers that stay silent after
  starting. Disabling, editing and removing a definition cancels hidden direct
  chats and roundtables before any frame or queue drain. Normal tests passed
  (1.115s), and the race run passed (4.188s).
- Small-package checks for ACP, agent settings and plugin manifests passed;
  settings documentation/schema checks passed after updating the usage guide.
- Final ACP, independent-chat, room, diff-LSP navigation, translation and motion
  checks passed (2.828s). The final ACP/chat integration race run passed (11.518s).
- `go vet ./cmd/fiveleaf ./internal/roundtable ./internal/acp ./internal/settings
  ./internal/plugin ./plugins` passed. The current workspace builds to
  `/private/tmp/fiveleaf-roundtable`. Go formatting and scoped whitespace checks
  passed. A missing closing brace in the concurrently edited icon-theme selector
  was restored to unblock compilation without changing its behavior.
- Usage is documented in `docs/AGENTS-ACP.md`, `docs/SETTINGS.md` and
  `docs/PLUGINS.md`; English, Chinese and Japanese labels are generated. Short
  user-facing entries were added to `CHANGELOG.md`.
  UI screenshots are in `/private/tmp/fiveleaf-roundtable-shots/` and agent
  configuration screenshots in `/private/tmp/fiveleaf-agent-config-shots/`.

  The full test suite is not run, per repository instructions. External agents'
  authenticated model calls are not exercised by these helper fixtures.
