# ACP review navigation and registration

Goal: use language-server navigation while reviewing changes, offer more ACP
agents, and register custom ACP commands inside FiveLeaf.

## Plan and acceptance

1. [x] Route diff definition/hover actions through the existing LSP client, preserving
   the review tab. Resolve the selected side's symbol against current source only
   when the location maps reliably; report an unavailable/stale location otherwise.
   Verify split/inline and immutable snapshots, UTF-16 positions, dirty current text,
   stale/deleted lines, local comparison paths, and an actual LSP request/response
   fixture. Discard replies after a tab, workspace or source-version change; pointer
   navigation must not trigger diff editing or hunk actions.
2. [x] Add a native custom-agent registration form to Settings → Plugins and link it
   from Agent configuration. Keep executable, arguments and environment separate,
   validate before saving, and support edit/remove using existing settings persistence.
   Verify validation, persistence, existing plugin overrides, and visible form states.
3. [x] Add optional bundled ACP agents using commands verified against their official
   docs/registry. Test exact manifest launches and keep processes opt-in.
4. [x] Document navigation behavior and new registration/agent setup, add translated
   UI labels and short Unreleased changelog bullets. Run the relevant LSP/diff/ACP,
   settings/plugin/UI checks and a build. Do not run the full suite.

## Scope and ownership

- Navigation worker: diff/LSP implementation and its focused tests.
- Registration worker: settings form and Agent entry points, form tests.
- Adapter worker: new bundled plugin manifests and plugin tests.
- Root: integration, shared locales/commands when needed, documentation and checks.

The existing FiveLeaf rename, themes, LSP improvements and other workspace changes
predate this task and must be preserved. There is no `.codegraph` index here.

## Evidence and decisions

- `goToDefinition` and `showHover` currently read only `textEditor()`.
- `agentServers` already supports arbitrary ACP executable/argv/env at user scope;
  `KindJSON` currently opens the settings JSON rather than a registration form.
- Agent snapshots are immutable; LSP must not overwrite a live document with old text.
- A fresh plan review identified missing checks for asynchronous navigation replies,
  pointer actions, and the peer path in local file comparisons; step 1 now includes
  them. Reusing the existing LSP client avoids introducing historical LSP sessions.
- Custom registration will use separate rows for each argument and environment
  variable, preserving empty values and spaces without shell parsing. New IDs are
  generated to avoid plugin collisions; edits keep their IDs and detect concurrent
  changes. The form also works when the plugin subsystem is unavailable.
- Added Gemini CLI 0.68.0, Qwen Code 0.27.3, OpenCode (installed locally), and Kimi
  Code 2.6.2 using verified official registry and release documentation. All remain
  optional. `go test ./plugins -run TestBundledACPAgentManifests -count=1` passed
  (1.661s); the complete small plugin bundle package also passed (0.453s).
- Shared regression checks passed for translated commands, copy rules, settings
  labels, motion tokens, Agent UI, permissions, ACP subprocess chat and terminal
  Agent sessions (1.541s). Settings schema, ACP validation and documentation checks
  also passed (0.654s). New navigation and registration tests are tracked below.
- All final checks use the original workspace without an overlay. The final
  `go vet ./cmd/fiveleaf ./internal/keymap ./plugins ./internal/lsp/...` and
  `go build -o /private/tmp/fiveleaf-acp ./cmd/fiveleaf` passed.
- Built-in navigation bindings now use `editorFocus`, including diff views.
  The keymap preset/platform checks passed (0.544s).
- Native registration and settings checks passed on the original source tree
  (9.540s): `TestPrefsAgent*`, `TestShotPrefsAgent`, `TestPrefsTexts`,
  `TestPluginsPage`, `TestPluginDetail`, `TestPrefsShowAndSearch` and
  `TestPrefsResetUndo`. The tests exercise real form input, persistence, empty and
  multiline arguments, environment values, concurrent edits, safe remove/reset
  undo, plugin overrides and configuration without a plugin manager.
- English dark (940px), Chinese light (640px) and Japanese dark (640px) registration
  screenshots were inspected; the form and Save/Cancel controls fit. Images are in
  `/private/tmp/fiveleaf-acp-prefs-shots/`.
- The combined Agent, translated-command, copy/motion, registration and plugin
  regression checks passed again against the current workspace (8.959s), with
  registration screenshots regenerated and the Chinese narrow layout rechecked.
- All diff-navigation and save-observer tests passed (2.744s). They cover real LSP
  subprocess requests, current unsaved text, UTF-16 columns, split/inline review,
  local comparison paths, stale replies, pointer actions and document lifetime.
- Closing reviews now follows the existing save/discard/cancel lifecycle for the
  last retained view. Waiting for a pending save does not request another write;
  discard decisions are checked against the buffer version again before closing.
  Tests cover shared documents, pending writes, errors/conflicts, cancellation and
  new edits during multi-document confirmation.
- Focused race checks covered navigation, save observers, existing save/conflict
  behavior, ACP write protection, diff selection and the language-server fixture.
  The dirty-buffer fixture now explicitly disables autosave; its three inline/split
  subtests passed under `-race` (16.321s). All other selected tests passed in the
  preceding race run, with no race reports.
- Independent review found that ordinary source queries also needed request-order
  protection. Hover and same-file definition replies now reject older query serials.
  Both new regressions failed before the fix and passed afterward; the related
  source/diff/LSP group passed (2.438s).
- Formatting and whitespace checks passed. The full test suite was not run, per
  repository instructions. External agents' authenticated model calls were not
  exercised; their manifests and official launch requirements were checked.
