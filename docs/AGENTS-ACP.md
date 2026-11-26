# Agent chat, roundtables and code review

FiveLeaf hosts external Agent Client Protocol (ACP) adapters and command-line
agents in its native MyGo interface. Each agent manages its model, account and
tools. FiveLeaf provides independent chats, project roundtables, editor context,
ACP permission choices and change review.

## Start a chat

1. Open a local project folder.
2. In **Settings → Plugins**, install and enable your agent's plugin. To register a custom program,
   open **Settings → Agent → Add Custom Agent…**. Sign in using the agent's usual CLI first,
   or use **Sign In** when the adapter advertises a supported login method.
3. Open **Sessions** in the sidebar and select **New Chat**. Use the arrow beside it
   to choose a registered agent.
4. Type a message and press `Enter` to send. `Shift+Enter` inserts a new line.

The composer and sidebar pickers show each agent's icon before its name. The right-hand column shows
live status: a circled check for online, a broken link for offline, a pulse while working,
and a warning triangle if the connection failed. Hover over a row to read its status.
The check on the left marks the selected conversation. Select a row to switch to its conversation and draft,
including while another agent is replying. **Settings → Agent** lists agents you added or
overrode and lets you enable or disable them. A badge on **Sessions** signals a pending ACP
permission. Right-click **Sessions** and select the chat marked **Permission required**.

**Sessions** is a separate sidebar activity for local history from Claude Code,
Codex, Pi, Qoder, Grok, plugins and roundtables. Open it with `view.sessions` without replacing
the chat in your workspace. Right-click **Sessions** in the sidebar to switch between open chat tabs.
Double-click an agent chat or choose **Resume** to continue in an editor chat in its original project.
Resuming a Codex conversation reopens its existing chat when the project and agent configuration match, including drafts and queued input.
An adapter that supports restoration keeps the original session ID across turns. Other agents use excerpts
from the local transcript as context. **Continue with…** lets you choose the agent that receives those excerpts.

Chats open in workspace tabs. The first chat toolbar button, **New Chat** (`+`),
opens another tab, and **Split Right** starts a separate chat beside the current
one. Both can use the same agent. Each tab takes its name from its first question
and keeps its own draft and replies. **Reopen Closed Tab** restores a recently
closed chat while the project remains open.

Opening a chat connects its ACP adapter automatically. Switching back to an open chat
reuses that connection. Neither a draft nor imported history is sent until you press
**Send**. The chat displays the adapter’s startup and reconnection output in the order it arrives.
CLI programs have no standing connection and start on the first message.
**Stop** asks the adapter to cancel the current reply. The connection stays open while the chat waits for the
original request to finish. Stopping during setup puts the unsent message back in the input and lets setup
finish. Choose **Reconnect** to restart an adapter while waiting. After an adapter exits, the next **Send**
also opens a new connection. A Codex adapter that supports restoration reopens the original thread with the
same configuration. The chat keeps its transcript and draft. Closing the project or app cancels all of its conversations.
If another Codex client still owns the thread, choose **Continue in a new thread** to keep the transcript and draft.
Sign-in choices appear only when the adapter reports that authentication is required.

Direct ACP chats wait for replies by default. A configured reply timeout counts time without progress after
the prompt is sent. New output or tool updates restart it, and waiting for an approval pauses it. When it expires,
the chat uses the same cancellation as **Stop**. Connection setup has a separate deadline. After an interruption,
the sent message and partial output stay in the transcript. Choose **Restore to input** on the original message
to edit its text and attachments before sending it again.

While a reply is running, **Queue follow-up** holds one message for this chat. It sends after the
current reply finishes normally. Stopping, an error or a disconnect pauses it. Use **Edit queued message** when
the input is clear to bring back the text and attachments, or remove it from the queue.

Replies render Markdown lists, tables, quotes and fenced code with syntax colors
from the current theme. Code blocks show their language, scroll long lines within
the block and offer **Copy code**. **Copy Message** preserves the original Markdown,
including text beyond the displayed limit. Roundtable replies use the same renderer.

Tool calls appear where they occur in the conversation. Consecutive calls share a disclosure so you can scan a
completed group or open each result. Failed tool details start expanded. Tool rows copy their full output,
including streamed command output and results sent separately by the adapter. Expanded calls show the input and
output available for that call. A command opens while it runs and folds when it finishes or later output arrives.
Change-review buttons remain available when details are folded.
Thinking appears in a distinct color once it has content. It expands while it streams and folds when the reply continues.
Use the arrow on the left to toggle its details, or choose **View reasoning** below a completed reply to return to them.
Reasoning uses a smaller, muted style than the public reply.
Codex labels progress updates separately and marks the final reply when the turn finishes.
Your manual expansion choices remain in place when new output arrives or you switch tabs.
Scrolling up keeps your reading position; **Jump to Latest**
returns to the newest output and resumes following it.

When an ACP agent sends a plan, **To-do list** appears above the input and updates as tasks progress.
The header shows how many tasks are complete. Select it to fold or reopen the list, which starts folded in short panes.

Open **Settings → Agent → Conversation settings** for the last focused chat in the project.
Edit its system prompt, choose how many recent messages to replay (1–100, default 10), or clear its transcript
without changing another split's chat. Replay includes user messages and public replies from finished turns.
Codex and natively resumed chats send follow-ups to the existing session, whose agent manages its own history.
For a regular Codex chat, the replay limit applies when connecting again or starting a thread with a changed system prompt.
Other new ACP chats replay recent messages in a fresh session on each turn. FiveLeaf applies the selected mode and model before sending.
An unavailable selection produces an error. **Clear Chat** clears
the selected transcript while retaining its draft, role and context settings.

Type `@` in the draft to search project files by name or path. Choosing a file inserts its name and attaches it to the message.
Start a draft with `/` to browse the current agent’s commands. Session commands appear before Pi’s `skill:` entries and Codex’s `$` skills. Search the whole list by name or description. Use the arrow keys and `Enter` or `Tab` to choose, or `Esc` to dismiss. Add any arguments, then send the command to the current session.

When an agent offers a model, thinking level or mode selector through ACP, `/model`, `/thinking` and `/mode` give you access to the corresponding choices. Select a value to apply it, or send the setting with its exact value, such as `/thinking high`. An advertised slash command with the same name keeps its agent-defined behavior. Pi’s `/settings`, `/tree` and `/scoped-models` open screens in the Pi terminal.

The input stays at the bottom of the conversation, including empty chats. It grows as text wraps and scrolls at its height limit.
Pasting more than 20 lines or 8 KiB at the end of the input adds a collapsible text attachment. Select it to preview
or remove it. Editing an attachment moves it and any earlier pasted text into the input in order. Sending includes the
complete remaining attachments after the typed message.
Permission requests stay above the input while you read older messages.
Its footer contains the model, Agent and send controls. Wide Agent
pages also show the workspace directory used by the ACP session; hover for its full path.

Open **Conversation details** with the speech bubble in the chat toolbar or detached title bar.
Drag anywhere on the card’s title bar to move it within the chat. It stays open while you type or scroll and
returns to the same position when you switch back to the chat. Use **×**, the speech bubble,
or `Esc` in the active chat to close it. **Changes**
shows added and removed lines for the current repository, not just edits made by this
agent. Select the row to open the repository diff.

Choose **Export as Markdown…** in the same card to save this chat’s sent messages and public replies.
The file includes loaded history and any reply text already received when the save dialog opens.

**Sources** belongs to the individual chat. It lists files and selections you added,
imported conversation history, and resources explicitly reported by the agent's tools.
Select an attached file to reopen it, or **View all sources** to expand the list. The
**+** and the file-plus button in the message field open the file chooser. You can also drop files on the
field or paste copied files and screenshots. Hover over a thumbnail or filename, then move into
the preview to scroll through the attached text or a tall image. Move away or press `Esc` to close it.
Text previews include unsaved editor changes.
ZIP files list their entries. PDFs show the first page with Poppler or macOS Quick Look, which also
provides previews for supported Office documents. Other binary files show their first bytes.
Use the × beside a filename to remove it. **Send** includes the text snapshot or the local path for
images, PDFs and binary files. You can send attachments on their own.

After connecting an ACP agent, open the model pill in the composer to choose from
the models it advertises. Search matches names, IDs, descriptions and groups.
The left rail filters by the adapter's groups, falling back to the provider prefix
in model IDs. **Recent** shows up to eight models selected in this chat, kept in
memory across **New Chat** and reconnects. Use the up/down keys to move through
results, `Enter` to choose and `Esc` to close; a check marks the current model.
The selection applies to that conversation and takes
precedence over its role's saved model. It stays selected across later turns,
**New Chat** and reconnects, without
changing the role or the agent's own defaults. A failed switch keeps the previous
selection and draft. Model changes are available when the conversation is idle.

The compact model button shows the model name; its tooltip retains the full ID.
The picker opens above the draft and stays open after a model is chosen, so its
thinking control remains within reach. For agents that advertise thinking levels,
the model picker includes a thinking intensity slider below the model list. Click
or drag to a level, or focus the slider and use arrow keys, `Home` or `End`. Only
levels supported by the current model appear, labelled in English; a provider
default appears as **Provider default**, and adapters that advertise `off` or
`auto` keep those choices. Each level has a distinct color; flowing highlights
pause when the picker is closed, the window is inactive or Reduce Motion is
enabled. Pi confirms the selected level before it is used, and that level is
reapplied after model setup for subsequent prompts. Other Agent modes stay in the
composer menu.

A thin progress bar sits on the input's top border and shows reported context
occupancy, with the used percentage above its left end and total capacity above
its right end, such as `1M`. Its ends
sit inside the input's rounded corners. Hover anywhere on the bar for exact token counts, or click it
to open **Context** details with exact token counts, percentages, category estimates
and free space. Free space is total capacity minus reported usage. Unreported
capacity appears as `—`. Values update when the adapter
reports them, including reports sent after a reply finishes, and reset for a new session or disconnect.
Until the first request, system prompts, skills and tool definitions are not included.
For Codex, this is the most recent request's input context without generated output.
For Pi, the capacity comes from `contextWindow` in Pi's model catalog, not `maxTokens`.

For Pi, the bar also marks the compaction threshold from the configuration loaded
for the current session. The default reserves 16,384 tokens; per-model overrides and
disabled compaction are respected. The marker follows the current model's capacity,
so a fixed percentage is not assumed. The details show the reserved buffer in a
separate section; it is not counted as used context. The summary also shows how
many tokens remain before compaction. Configuration is refreshed when a new
Pi session starts, including follow-up turns and reconnects.

Codex handles automatic compaction on its active thread. **Context** details show compaction reported by Codex.
Use `/compact` when the adapter advertises the command to compact the current thread on demand.

ACP currently reports the total occupancy without a full category breakdown.
The details estimate visible conversation content, show unavailable internal
categories as **Not reported**, and retain the remainder as **Unclassified Context**.
These estimates are separate from task progress and cumulative token billing.

## Other ACP agents

Each of these connections has an [Agent plugin](#agent-plugins). For a custom registration, use the ACP protocol and the same launch command.

| Agent | Command | Arguments |
| --- | --- | --- |
| OMP | `omp` | `acp` |
| Codex | `npx` | `-y @agentclientprotocol/codex-acp@2.7.0` |
| Claude Code | `npx` | `-y @agentclientprotocol/claude-agent-acp@0.107.0` |
| DSH | `dsh` | `--profile acp` |

OMP and DSH provide ACP directly. The Codex and Claude Code adapters require Node.js;
their first connection may download the adapter. Codex uses an installed CLI when
available, otherwise the adapter's bundled CLI. An explicit `CODEX_PATH` takes precedence.
The Claude Code adapter includes its CLI. Each connection uses its agent's existing
authentication and model-provider configuration; a DSH ACP profile can differ from its
desktop profile.

Model and thinking choices come from the connected adapter. A provider-default thinking
choice is preserved even when its protocol value is empty. Context percentages use the
adapter's reported occupancy and capacity, which can differ between agents using the
same model. Until usage arrives, the indicator stays unknown. Agents that do not report
a compaction policy do not inherit Pi's threshold or reserved capacity.

An optional live check exercises these adapters through the FiveLeaf UI. Run
`go test -tags agentintegration -run '^TestAgentLiveWorkflow$' ./cmd/fiveleaf`
with `FIVELEAF_AGENT_LIVE_CONFIG` pointing to a private JSON file containing an
`agents` array. Each entry supplies `id`, `name`, `server`, two available model IDs
(`model` and `alternateModel`), `replyMarker` and `thoughtHigh`. Set `restoreDefault`
to check an empty provider-default thinking value across two replies. Use isolated
agent state in each server's environment. This check makes real model requests in
an empty temporary workspace and fails if an agent requests tools or permission.

## Configure and use roles

In **Roundtable → Invite members**, choose a built-in or saved role. An unassigned built-in
uses an available Agent. Assign an Agent in settings to use a specific one.
If a CLI's arguments contain `{model}`, assign that CLI and a model ID to the role.

| Built-in role | What to ask for |
| --- | --- |
| Code Explorer | Trace an entry point to the relevant files and symbols without editing. |
| Planner | Break a change into ordered steps with an acceptance check for each, without editing. |
| Implementer | Make a scoped change and report which checks passed. |
| Test Engineer | Cover normal and failing cases without changing production code. |
| Code Reviewer | Identify regressions with file locations and triggering conditions, without editing. |
| Diagnostician | Separate evidence from hypotheses and suggest the next check, without editing. |
| Documentation Writer | Update documentation and check its examples against the implementation. |
| Requirements Analyst | Turn a request into observable acceptance criteria for a typical case and an edge case. |
| Interaction Designer | Walk through a task with the keyboard and in a narrow window. Identify where users get stuck. |
| Compatibility Maintainer | Check an upgrade and recovery on a copy of older settings and plugins. |
| Roundtable Moderator | Summarize agreements and disputes, then hand off a question or propose the next check. |

Each member receives its role prompt with the roundtable conversation.

Open **Settings → Agent → Role List**, or choose **Configure Roles…** beside **Invite members**. Use the pencil
beside a role to edit its Agent, model or prompt, then choose **Save**. The Agent choices include
enabled plugins. Choose an advertised model, use **Load models** to query an ACP adapter, or enter
a model ID. An empty model uses the Agent's default.

The list previews the first line of each role’s effective prompt. **Add Role…** starts a named role.
**Copy Role** opens an editable copy of the selected role’s prompt, Agent and model, including the
**Current agent** choice. Choose **Save** to add the copy to the list and roundtable invitations.

**Restore defaults** on a preset restores the current Agent, its default model and its localized
prompt. **Restore all preset role defaults** restores every preset in the list. Both actions offer **Undo**.
You can edit, rename and remove your own roles in the same list.

**Export Roles…** and **Import Roles…** share roles between profiles. An Agent can also batch-create
roles with `ff roles import -` or the [local role API](SETTINGS.md#share-roles-and-create-them-from-an-agent).

ACP roles select the requested model before sending, including on fresh sessions
used for later turns. The adapter must advertise model selection through session
configuration options or its legacy model list. An unsupported model or failed
selection stops the request and keeps the draft.

For a CLI Agent, configure its normal model flag with a literal `{model}` in the
argument array, for example `args: ["--print", "--model", "{model}"]` for a
compatible CLI. The placeholder is replaced in arguments only, without a shell.
An explicit role model without this placeholder produces an error before the
command starts. An empty role model with a placeholder also stops before starting.
Use a separate CLI Agent definition without the placeholder for roles that should
keep the Agent's original model arguments or default model.

Editing a role disconnects its previous configuration while retaining history and
drafts. Missing or disabled Agents leave their roles visible and unavailable.
Renaming a saved role in `settings.json` creates a new invitation target. Earlier messages
retain their original speaker names.

## Hold a roundtable

Choose **Sessions → Roundtable** in a local project to open a meeting tab beside your files. The Sessions sidebar shows the project’s meetings. Select a row to switch meetings, or choose **New roundtable** to enter a topic and invite members. The optional brief holds your question, constraints or expected result.
**Create and start** assigns the first task immediately. **Create roundtable** opens the meeting with the brief in the composer for editing. Saved meetings appear alongside chats in **Sessions**.
Choose **Use a template…** to prefill members and a brief for design review, incident diagnosis or implementation verification. The form keeps your topic, chair and any brief you have edited. You can adjust the brief and invitations before creating the meeting. Each role uses its configured Agent and model.
Choose **All Agents → Meeting records** to see only meetings. Search by topic or member,
then double-click a meeting or press `Enter` to open its transcript.

Invite built-in roles, custom roles or individual Agents. Two roles using the same
Agent speak as separate members, each with its own instructions and model. The roster
shows the Agent behind each role. Members receive the topic, the roster and recent
public messages from this group. ACP members retain editor permissions and tool
diffs. CLI members return their captured output.

Choose who chairs the meeting when creating it. Choosing the Roundtable Moderator invites it to read the topic, assign work to members and review their findings. Hover over the badge below the meeting name to see who is chairing.

To chair the meeting yourself, open **Collaboration** and select the human chair. Your messages become meeting remarks. Use `@` to give a member the floor for a single reply, or publish your own conclusion. Between turns, right-click the moderator and choose **Remove from group** to take the chair. Choosing the moderator in **Collaboration** invites it back if needed. Each meeting remembers its chair when reopened.

For a direct question, type `@` in the composer and choose a member. Filter the list by name or ID, then press `Enter` or `Tab`, or click the member. The picker distinguishes people with the same name by ID. You can also start the question with an exact `@name` or `@id`.

The conversation shows your messages on the right with **You** above them. The moderator’s messages appear on the left with an **AI chair** label. A conclusion you publish adds **Human chair** next to **You**. Expand long replies in place and open an image or Mermaid diagram for a closer look. The **Member panel** lists the participants beside the chat. Select a member to inspect each assignment, its timing, output and tool activity. Open the panel from the header when the editor is narrow. **Collaboration** controls how long replies appear. You can invite another role between turns.

Open **Group memory** to see the recent public remarks and answers members can use. Pin a decision or constraint to keep it available beyond that window. New meetings share the latest 12 turns. The panel also offers 30 and 50 turns. Newly invited roles use the same group memory. Task details show how many previous turns and pinned notes the member received.

The pinned strip above the conversation previews the first three notes. Select the strip to open **Group memory**.

Choose **Execution Graph** to see the recorded tasks in chronological order across member lanes. The matrix counts transfers between consecutive speakers within a turn. Select a node, a transfer count or a reply’s author to read the assignment, timing, supplied memory, tool calls and Markdown output in the member panel.

Pausing sharing keeps the notes available in the panel. Members then receive the current question, assignment and findings for the current turn. **Clear group memory** starts the next turn with fresh context. Earlier messages and task records remain available in the meeting transcript and Markdown export.

Choose **Export meeting** to save a Markdown file with the complete archived conversation, assignments and recorded tool details, including earlier pages. Export is also available when reading a meeting from another project.

On a conclusion card, choose **Continue with an Agent…** and pick a recipient. A new chat in the meeting's project receives the question, conclusion and available evidence references from that turn. Review the draft and edit the next request before sending it.

Write at any time to start a new turn. Switching meetings or opening a file lets active discussions continue. The sidebar marks a background meeting that needs an approval. **Stop** ends the displayed turn. Closing the meeting tab stops the project’s active discussions. FiveLeaf saves accepted messages in its
local application data directory, grouped by project folder and topic. Reopening
a group restores its transcript and settings and waits for your next message.
Use **Earlier messages** to read beyond the recent model context. **Clear chat**
asks for confirmation before removing the group’s saved messages.

Invitations keep the selected Agent binding. If a role is removed, reassigned or
its Agent is disabled, its earlier messages stay readable under the original name.

## Agent plugins

In **Settings → Plugins → Agents**, search for the agent by name, install it and turn it on.
You can also install from the CLI, for example `ff plugin install agent-omp`. Existing plugin settings
and roles use the same IDs after installation. See the [launch commands](PLUGINS.md#acp-agent-servers).

A plugin's switch stays off until FiveLeaf finds the local Agent CLI and its launcher.
Install the command shown in the warning, then choose **Plugin Actions → Reload Plugins** to check again.

| Plugin | Setup before connecting |
| --- | --- |
| **Pi Agent** | Install Node.js 22 or newer and Pi. Sign in and select a model in Pi. |
| **OMP (Oh My Pi) Agent** | Install `omp`, configure a provider and check that your version provides `omp acp`. |
| **DeepSeek TUI (DSH) Agent** | Install `dsh` and configure the provider used by `dsh --profile acp`. |
| **Codex Agent** | Install Node.js and sign in with Codex using its normal CLI. |
| **Claude Agent** | Install Node.js and sign in with Claude Code using its normal CLI. |
| **Gemini CLI Agent** | Node.js 20 or newer. Gemini CLI 0.68.0 supports the documented Google Cloud / Code Assist organization login, Gemini API key, or Vertex configuration. Follow the version's account requirements; personal Google sign-in is not assumed. |
| **Qwen Code Agent** | Node.js 22 or newer. Start Qwen Code 0.27.3 interactively and use `/auth`; configure the supported subscription login or API provider. |
| **OpenCode Agent** | Install OpenCode from its official site and make `opencode` available on `PATH`. Start it and use `/connect` to configure a provider, then `/models` to choose a model. |
| **Kimi Code Agent** | Install Kimi Code, sign in using its CLI and check that `kimi acp` is available. |

Sign in and configure model access through each agent's own CLI. Pi, Codex and Claude Code
download a pinned Node.js adapter on first connection. The other plugins run the installed CLI directly.
If Codex disconnects during initialization and npm reports a missing `package.json` under `_npx`,
update the Codex plugin and reconnect. For a custom registration pinned to `2.6.0`, change its adapter
argument to `@agentclientprotocol/codex-acp@2.7.0`.

Sources checked for these presets: the [official ACP registry](https://github.com/agentclientprotocol/registry),
[Gemini CLI 0.68.0](https://github.com/google-gemini/gemini-cli/tree/v0.68.0),
[Qwen Code authentication](https://github.com/QwenLM/qwen-code/blob/v0.27.3/docs/users/configuration/auth.md),
[OpenCode ACP](https://opencode.ai/docs/acp/), [OpenCode providers](https://opencode.ai/docs/providers/),
and [Kimi Code](https://github.com/MoonshotAI/kimi-code).

## Use Pi

Install Node.js 22 or later and Pi, then sign in and configure a model in Pi. Install
`agent-pi` and enable **Pi Agent** in **Settings → Plugins**. The plugin runs `npx -y pi-acp@0.0.34`.
If Pi is outside the app's `PATH`, set `PI_ACP_PI_COMMAND` to its absolute executable path in
FiveLeaf's environment. You can also continue using an existing custom Pi registration with its roles.

Select Pi in the chat, then choose a model from the composer's
model picker. To save a role's default model, edit the role in **Settings → Agent → Role List**
and choose **Pi** and an advertised model ID in the form `provider/model`.
Credentials stay in Pi's own configuration; FiveLeaf does not copy them into its settings.

Pi reports context usage after session setup, model changes and completed replies.
The indicator follows those reports; it does not update for each streamed token.

Pi's built-in tools execute under Pi's own rules. The adapter forwards Pi
extension confirmations as ACP permission cards; it does not automatically ask
FiveLeaf to approve every built-in tool.

Pi confirmations show the question with localized **Yes** and **No** choices. **Request details**
opens the data behind the question. Tool requests show a command, a file location, or their detailed
input when more information is needed. Options that apply to future requests have their own row.

The optional live smoke test uses a real Pi and provider, an isolated Pi config,
and a fixture-only permission extension. Run it explicitly with `-tags
piintegration`, `-run '^TestPiAgentLiveWorkflow$'`, and
`FIVELEAF_PI_SMOKE_CONFIG` pointing to a JSON file with `model`, process `env`, and an optional `alternateModel`.
Both models must be available from the configured provider.
The fixture extension must confirm writes to `pi-smoke.txt` with `ui.confirm`
and block other tools. The test exercises registration, connection, model switching,
reported context, replies, follow-up context and returning from the editor to
a pending permission in the same chat.

## Use another adapter

Open **Settings → Agent**, or choose **Configure Agents…** in the command palette, then choose
**Add Custom Agent…**. Enter a name and an executable path or command name. Add
each argument in its own field, and environment variables as separate names and
values. Choose **ACP** or **CLI**, and optionally set its working directory,
capability tags and reply timeout. For example, an installed OpenCode uses ACP,
executable `opencode` and argument `acp`; a one-shot Claude CLI uses CLI,
executable `claude` and argument `--print`. Save to make it available in the Agent menus.

The form assigns an unused ID. Editing keeps that ID; removing a definition
offers **Undo**. Saving merges other definitions changed meanwhile, and reports
a conflict if the same definition changed while its form was open. Saving does
not launch the agent. Existing plugin overrides remain editable and are labeled
when their ID matches a plugin agent.

**Edit in JSON** remains available for advanced configuration. The equivalent
user setting is:

```json
{
  "agentServers": {
    "opencode": {
      "name": "OpenCode",
      "command": "opencode",
      "args": ["acp"],
      "capabilities": ["code", "reasoning"]
    },
    "claude-cli": {
      "name": "Claude CLI",
      "protocol": "cli",
      "command": "claude",
      "args": ["--print"],
      "workingDirectory": ".",
      "capabilities": ["code", "reasoning"],
      "timeoutSeconds": 120
    }
  }
}
```

`command` is an executable; `args` is an argument array. No shell is involved.
Use an absolute executable path if the GUI's `PATH` does not contain the adapter.
Optional `env` entries override the inherited environment. Keep authentication
in the adapter's normal credential store. Folder settings cannot add executable
definitions. Enabled plugins can declare `contributes.agentServers`; see
[PLUGINS.md](PLUGINS.md#acp-agent-servers).

Omitting `protocol` keeps ACP compatibility. CLI sends a formatted recent-message
prompt on stdin and captures stdout, with bounded stderr available for failures.
Set `promptArgument: true` for a CLI that expects the prompt as its last argument.
The program must support a non-interactive request and exit after its answer.
`timeoutSeconds` accepts 1–3600 seconds, or `0` for the default. Direct ACP chats have no default reply timeout.
CLI jobs and roundtable turns default to 120 seconds. On Unix, stopping or timing out a CLI job terminates its
process group, including child processes.

An empty working directory uses the project; relative paths resolve from it.
For ACP, this sets the process directory while the session and editor file
callbacks stay rooted in the open project. Capability tags affect speaker routing;
they grant no editor permissions. `disabled: true` pauses participation and
disconnects an active agent.

**Import…** and **Export…** in Agent settings exchange JSON configuration:

```json
{ "version": 1, "agentServers": { "claude-cli": {
  "name": "Claude CLI", "protocol": "cli",
  "command": "claude", "args": ["--print"]
} } }
```

Import validates the whole file, merges new IDs, accepts equivalent existing
definitions and rejects conflicting IDs without partially applying the file.
Export includes custom definitions and plugin overrides, with environment values,
and writes a file with private permissions. Importing does not start a process.

Agent-only exports use `version: 1`. Named roles assigned to an Agent use `version: 2` and `agentRoles`.
Preset overrides use `version: 3` and `agentRolePresets`. An export containing a named role that follows
the current Agent uses `version: 4`. FiveLeaf imports all four versions.
All maps merge in one update; a conflicting Agent ID, role name or built-in override rejects the
whole import. Unsaved Agent or role forms must be saved or cancelled before using
import or export.

Changing an active definition or disabling its plugin disconnects that adapter,
including in a background project. The draft remains available for reconnection.
Existing terminal Agent sessions remain in **Sessions** and can still be resumed.

## Add code and review comments

- The tab and composer show the chat's Agent icon, including when a role chooses the Agent.
  Chats use the robot icon until they have an Agent with a known icon. The chat that receives code selections
  has a solid badge behind its icon; other chats show the icon without a badge.
  Switch tabs or focus another chat split to move the badge. Returning to the editor
  keeps your last chat as the destination, even if two chats use the same Agent.
- Right-click selected code and choose **Add Selection to Agent** to put the exact
  text, file path and line numbers in the draft. With no selection, the editor
  offers **Add File to Agent** instead. Its attachment captures the whole file, including unsaved edits.
- `Option+Cmd+L` on macOS or `Shift+Ctrl+L` elsewhere adds the selection, or the
  current line if nothing is selected. Explorer files still offer **Add File to Agent**.
  A file still loading must finish before sending.
- Select lines in a diff and use **Add Selection to Agent** to include both added
  and removed lines with their source information.
- From a chat, use **Add Selection to Agent** or **Add File to Agent** in the command
  palette to add context from the last editor or diff you focused.
- Choose **Add Review to Agent** in the review toolbar or command palette to move your
  review comments into the draft. **Send to Terminal** remains available.

These actions only prepare the draft. **Send** submits it. Context is sent as
plain text with paths and fenced source, compatible with agents that do not
advertise embedded resources. A draft is limited to 2 MiB. The transcript displays
up to 32 KiB per message and clearly labels a truncated preview.

## Review changes

Tool activity shows progress and files reported by the agent. Clicking one opens
an immutable before/after diff in the editor, including in projects without Git.
The same diff supports line selection and review comments. It has no stage,
discard or accept actions that could apply an old snapshot to the current file.

For writes through the editor, the before-image is captured while preparing that
write, including any changes already saved by the user. A diff reported by
an agent represents the text the agent supplied; it can be a proposal, so verify
the current file before applying further work. Multiple writes can have separate
snapshots. These snapshot tabs are not restored as Git tabs after a restart.

**Git → Review Workspace Changes** opens the existing Git diff, with current staged and
unstaged changes, hunk actions and comments. It includes all workspace changes,
including edits made before the conversation or directly by the adapter's tools.

## Navigate code during review

Choose a symbol in a diff and press `F12` with the default keymap, use **Go to Definition** from its context
menu, or `Cmd`-click on macOS / `Ctrl`-click elsewhere. **Show Hover** in the context
menu shows language-server information for the same symbol. Enable the language's
server plugin and install its required server first, as for the ordinary editor.
Other keymaps use their configured definition and hover shortcuts.

Navigation works in split and inline diffs, including Agent snapshots. It queries
the current workspace and includes unsaved editor content. For an older version,
FiveLeaf maps the selected line only when it is unchanged and maps unambiguously
to the current file.
If that line was edited or removed, the review stays open with a message asking
you to choose the symbol in the current file. Historical text is never sent as a
replacement for the current document. Opening a definition keeps the review tab
available so you can return to your comments.

## Supported protocol and boundaries

The client uses stable ACP v1 over newline-delimited stdio JSON-RPC. It supports
session creation, adapter-advertised `session/load` and `session/resume`, streamed text and thoughts, tools, plans, offered modes,
agent-managed login, permission requests and cancellation. Agent slash commands
can be typed into the composer. MCP server configuration and automatic registry installation are
outside the current client.

File reads use the current editor buffer. Writes reject a document that already
has unsaved changes or a pending save. FiveLeaf prepares a temporary file in the
background, then checks on the editor's main thread that any originally open
document is still at the same path and has no new user changes. Only then does it
atomically replace the file. Typing, closing the document or using **Save As**
during preparation rejects the agent's write and preserves the user's text;
any queued user save can then proceed. File callbacks require a known session,
stay within the resolved workspace, reject Git metadata and non-text files, and
enforce size limits. These checks govern editor callbacks; the adapter runs as a
normal local process and may use its own tools.

Client terminal execution and terminal-based login are not advertised. Agents
with their own terminal tools can use those. Remote SSH workspaces show an explicit
local-folder requirement. Errors and rejected permissions remain visible.

CLI and ACP both implement the UI-independent `internal/roundtable.Adapter`
contract, `Call(context.Context, []Message) (Message, error)`. HTTP or gRPC adapters
can implement the same interface later. Scheduling and subprocess I/O run outside
the UI thread. Configuration is persistent JSON; transcripts are currently kept
in memory. SQLite history, an LLM judge and token budgets are optional extensions
and are not enabled in this version.

Automated checks exercise real helper CLI and ACP processes, including routing,
timeouts, cancellation, permissions, file edits and conversation isolation. They
do not make paid model calls or verify external accounts unless one of the optional
live checks above is explicitly enabled.

References: [ACP protocol](https://agentclientprotocol.com/protocol/v1/initialization),
[Zed external agents](https://zed.dev/docs/ai/external-agents),
[Zed Agent panel](https://zed.dev/docs/ai/agent-panel).
