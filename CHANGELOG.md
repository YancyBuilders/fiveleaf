# Changelog

The update window shows the section of the version it offers. Every release
needs a `## <version>` section here, or its build fails.

## Unreleased

- The Agent input has a clearer caret and grows as you paste or type multiple lines.
- ACP replies keep running while the agent reports progress or waits for your approval.
- Interrupted ACP replies stay in the transcript with **Restore to input** for the original draft.
- Codex keeps the same thread across replies so it can compact context automatically.
- **Context** details show when Codex starts, finishes or fails to compact the conversation.
- Copying a user message from restored Codex history preserves the original text.
- Agent chats skip empty thinking rows and group consecutive tool calls.
- Agent thinking uses a distinct color and a small disclosure arrow on the left.
- Codex chat labels progress updates and the final reply separately.
- Stopping an ACP reply keeps the adapter running while it cancels.
- Use **Reconnect** to restart an ACP adapter that is still waiting to confirm a stop.
- ACP chats wait for quiet replies by default. You can set an inactivity timeout for an agent.
- Reconnecting a Codex chat restores its original thread when the adapter supports it.
- Preview or edit large pastes without filling the Agent input.
- Queue one follow-up while an ACP reply runs. Stopping pauses the queued message.
- ACP permission requests stay above the input while you read earlier replies.
- ACP chats explain empty replies, refusals and output limits in plain text.
- Resume ACP sessions with adapters that require an empty MCP server list.
- Context usage keeps reports sent after a reply finishes, hides Pi's empty pre-request zero, and uses Pi's context window instead of its output limit.
- Resuming Codex from **Sessions** reuses its existing chat and keeps the draft.
- Tool details show streamed command output and results sent separately by the adapter.
- Use **View reasoning** below a reply to reopen the reasoning the adapter shared for that turn.
- Running commands open while they work, then fold when they finish or the reply continues.
- Reasoning uses smaller muted text, folds when the reply finishes, and opens on request.
- Avoid two local adapters taking one Codex thread, and explain when another Codex client owns it.
- Show sign-in only after the adapter reports an authentication error, not merely because it offers login methods.

## 0.1.0

The first FiveLeaf release, based on Mayoi.

- Mayoi plugins keep using their original Mayoi version requirements.
- Agent writes preserve files saved or created by another editor during the write.
- FiveLeaf disconnects an Agent whose connection stops accepting requests.
- Closing a Diff asks again when text changes while the confirmation is open.
- The tool picker stays current when plugins are installed or removed.
- Markdown table edits stay responsive while images load.
- Rerunning a task keeps the file and cursor from its original run.
- Roundtables stay responsive while rebuilding daily memory from long histories.
- Updating plugins preserves the versions you pinned.
- Update Git plugins while the marketplace is unavailable.
- Installing a catalog plugin from a Git branch uses its listed folder.
- User settings are saved with owner-only file permissions on Unix.
- Read and clear old Markdown project settings with `ff settings --folder`.
- Check for updates from the upper-left corner, where an icon highlights new versions.
- Read release notes, download the update and restart from the update dialog.
- Update to a supported version when FiveLeaf requires a newer release.
- Agent chats show a foldable **To-do list** above the input when the agent sends a plan.
- Use **New roundtable → Use a template…** to prefill members and a meeting brief.
- Choose **Continue with an Agent…** on a roundtable conclusion to draft a follow-up.
- Roundtables show who sent each message above it, with a label for the AI chair.
- The **Execution Graph** counts handoffs within each roundtable turn.
- The sidebar's **Agent Sessions** button uses the robot icon.
- Right-click to remove chat history or delete a roundtable record.
- Click a date heading in **Agent Sessions** to collapse or expand that group.
- Headings, emphasis, links and task checkboxes keep their formatting while you type in Live Preview.
- Click or drag across the **Insert Table** grid to choose the number of columns and rows.
- Add or delete rows and columns from the cell’s context menu or the **Table** button while editing.
- Tables, diagrams and display math stay visible while edits refresh a split Markdown preview.
- Use `Tab` and `Enter` to move between Markdown table cells.
- Choose a code block’s language in its header to apply syntax highlighting.
- Use `⌘1`–`⌘6` for Markdown headings and `⌘0` for a paragraph, or `Ctrl` on Windows and Linux.
- Mermaid flowcharts use rounded decisions, thin arrows and compact branch labels.
- Mermaid flowcharts tint entry paths and highlight decisions in amber.
- Dark Mermaid flowcharts use blue, teal and rose for entry paths and graphite for shared steps.
- Mermaid sequence, class and state diagrams use blue cards in light and dark themes.
- A split Markdown preview keeps its reading position while you edit the source in another pane.
- Large Markdown tables scroll more smoothly while a cell is being edited.
- Markdown opens in editable Live Preview by default.
- Use the **Source** or **Preview** button beside a Markdown file’s path to switch its view.
- Long tables in Live Preview keep the headings and text below them clear as you scroll.
- Format Markdown from the keyboard with shortcuts for bold, italic, code, links and images.
- Press `Enter` in a numbered list to renumber the consecutive items that follow.
- Press `Backspace` at the start of a Markdown item's text to remove its list or quote marker.
- Drag or `Shift`-click in Live Preview to select the text under the pointer.
- Toolbox tools open in editor tabs and keep their input when you switch tabs.
- Timestamp, Base64, JSON and URL show their original DevTool layouts, effects and feature tabs.
- Create a Vue or React tool with `ff plugin new name --tool=vue` or `--tool=react`.
- Install Timestamp, Base64, JSON and URL independently in **Settings → Plugins → Tools**.
- `ff plugin new name --debug-adapter` creates a working debugger plugin for a new file extension.
- `F5` lets you choose when several enabled debuggers handle the same language.
- JavaScript and TypeScript breakpoints and stepping follow the Node.js target in **Run and Debug**.
- Hover beside a line number to preview a breakpoint. Click to add or remove it.
- During debugging, hollow breakpoints show their verification status on hover.
- `ff plugin new name --tool` starts a Toolbox tool with its page and theme calls.
- Debug adapters can resolve their command, transport and launch from the workspace at start.
- Debug adapters that declare `resolve` start on the workspace host and show **Starting** while they wait.
- **Run and Debug** starts a DAP adapter for Go, Python, C/C++, Rust, JavaScript and TypeScript.
- Click the editor's margin or press `F9` to toggle a breakpoint; step and continue from **Run**.
- The Debug view shows the call stack, variables and the program's output.
- Tools now open from the command palette, which lists every enabled tool under **Toolbox**.
- The **Toolbox** button opens that palette with the tools alone; type `$` to list them without it.
- **Settings → Appearance → Layout → Hide Toolbox Button** takes the Toolbox button out of the sidebar.
- Plugins have one category and their own version, instead of tags standing in for both.
- **Settings → Plugins** filters by category and shows the version of each plugin.
- Install a version with `ff plugin install name@version`, and list newer ones with `ff plugin outdated`.
- Updating a plugin takes the version the index lists, not the last commit of a branch.
- Markdown now has only source and editable Live Preview, without a read-only Reading view.
- Markdown editing no longer shows a formatting toolbar above the text.
- Right-click Markdown to format, insert links, images, code, tables, lists, tasks and quotes.
- Undo and Redo sit at the top of the Markdown right-click menu.
- Yazi shows its theme colors and selection highlight when FiveLeaf starts from a plain-text launcher.
- Yazi's folder and file icons use an installed Nerd Font when the chosen terminal font lacks them.
- The terminal cursor keeps the task strip's selection color, so Yazi's selected row and its cursor are both visible.
- A task's editor tab closes after a successful exit; a failed task keeps its output.
- Tasks open in their own editor tabs and run independently of the terminal panel.
- Assign, change or clear a shortcut for any task in **Settings → Terminal → Tasks**.
- **Open yazi** highlights the active file in its folder.
- Mica’s background flows in soft, curling bands of color under the editor glass.
- With Mica on, the FiveLeaf icon appears when you open an Agent chat or a roundtable conversation.
- In dark chats, silver FiveLeaf particles scatter around the pointer and settle back into the leaf.
- The Mica sidebar and Agent input share a frosted surface and sit closer to the window’s lower edge.
- Tabs, notifications and roundtables share the window’s Mica glass and moving background.
- **Settings → Terminal → Tasks** shows each task’s source and opens global or project task files.
- Type `@` in Agent chat to search and attach a project file.
- Type `/` to choose a skill or command offered by the current ACP agent.
- The `/` menu puts session commands first and includes every skill the agent offers.
- Use `/model`, `/thinking` or `/mode` to change settings offered by the connected agent.
- **Execution Graph** opens with task nodes already sized for the meeting pane.
- Hover over the badge below a meeting name to see who chairs it.
- Removing the moderator hands you the chair. Choosing the AI chair invites the moderator back.
- Return to **Agent Sessions** from the meeting list while keeping the current roundtable open.
- Address a member with `@` or search project files to insert a reference in a meeting message.
- Press `Enter` to send or `Shift+Enter` for a new line in roundtables and Agent chats.
- Open each day's pinned notes and conclusions as Markdown from **Group memory**.
- Right-click a member card and choose **Remove from group** to remove an invited member.
- **Execution Graph** gives the graph the full pane and lets you reopen the member panel.
- Open a node or handoff to inspect its inputs, tools and output, then step through nearby tasks.
- The member panel shows each member’s latest task, status and meeting totals.
- Open **Group memory** or **Collaboration** from the meeting header.
- Meeting conversations group topics and show delegations, evidence and the final decision.
- Role lists and meeting messages identify each role with a small color dot.
- In **Execution Graph**, follow handoffs and select a task to see its tools, memory and output.
- Preview the first three pinned notes at the top of a meeting conversation.
- Choose who chairs a new roundtable. An AI-led group automatically includes the moderator.
- Pin decisions and constraints in **Group memory** for members to use in later turns.
- Choose how many turns members recall, pause sharing, or clear group memory while keeping the minutes.
- **Roundtable** opens in an editor tab, with the project’s meetings in the Sessions sidebar.
- Choose a meeting from the sidebar, then open its member panel to inspect tasks or invite a role.
- Meetings keep working when you switch to another meeting or file in the project.
- The **Roundtable** moderator assigns follow-up work until it has an answer.
- Read questions on the right and replies on the left, with Markdown, images and Mermaid diagrams.
- Type `@` to choose a meeting member, or use **Export meeting** to save the full Markdown record.
- Start a **Roundtable** from a topic and receive a conclusion from the invited roles.
- Select a meeting member to inspect each assignment, result and tool activity.
- Roundtable replies omit Pi startup inventories, including those in older meeting records.

- **Settings** opens on **General** to match the first sidebar item, and **Appearance** splits into **Theme**, **Window**, **Material** and **Layout** cards.
- **Settings → Appearance → Material** folds its sliders until the material is on, with **Tune parameters…** to open them first.
- Claude Code folders and Agent exclusion files move beside the Agent and Copilot settings that use them.
- **Settings → Plugins** groups each language with its language servers under **Languages**.
- Plugins can declare debug adapters, which show in their details and under **Settings → Plugins → Debug adapters**.
- Search roundtables by topic or member in **Sessions → All Agents → Meeting records**.
- Built-in roles sit under **Invite members** without a separate heading.
- Open the **Commit Graph** from Source Control and filter history by local or remote branch.
- The commit graph now appears only in the editor, leaving more room for changes in **Source Control**.
- Enable Timestamp, Base64, JSON and URL tools individually in **Settings → Plugins → Toolbox**.
- Open **Toolbox** beside **Sessions** to search for tools in a compact picker.
- Search for a tool or feature such as `Base64 decode`, or open **Manage Plugins** from the picker.
- Toolbox pages show the tool and its feature tabs, including Timestamp, Time Zone and Calculator.
- The built-in Toolbox pages follow the editor theme, including file and JSON views.
- **Toolbox → Timestamp** keeps the first card visible and lets you scroll through every time zone.
- **File → Compare Two Files…** now sits beside **Open Recent** and compares files in any folder.
- In the Explorer, use **Select for Compare** on one file, then **Compare with Selected** on another.
- Install VS Code, JetBrains and Sublime Text keymaps individually in **Settings → Plugins**.
- Install Agent plugins individually, including Pi, OMP and DSH, in **Settings → Plugins**.
- Install individual language servers, formatters, Word Count and TODOs from the plugin catalog.
- Codex uses an updated ACP adapter, and Agent disconnects show which request failed.
- Agents that require authentication now prompt you to sign in or configure provider credentials.
- Open folders with `ff .` and install plugins with `ff plugin install <name>`.
- The sidebar and chat input sit slightly closer to the status bar.
- **Settings** and **Roundtable** open centered over the project window you opened them from.
- The plugin catalog stays searchable when the marketplace returns a server error.
- Agent plugins show the missing executable and stay off until it is available.
- Find and install individual themes like One Dark Pro and Material Icon Theme in **Settings → Plugins**.
- Installing a theme or icon set restores the selection you saved with the old bundles.
- Reconnect sits beside Send in the chat composer and becomes an icon in narrow panes.
- Sidebar activity tips open below their buttons, clear of native window controls.
- FiveLeaf includes a light and dark theme with a rounded sidebar and bars that match the editor.
- Folder, Git and remote connection actions fit their labels in empty views.
- Agent setup pairs a tinted **Configure Agent** button with an outlined **Manage Plugins** button.
- Settings actions use compact buttons with icons for adding, configuring and importing items.
- Code suggestions appear beside the caret. Use `↑` and `↓` to choose, then `Tab` to insert.
- JavaScript and TypeScript member suggestions stay visible as you type after `.` or `?.`.
- JavaScript and TypeScript look up members from imported files on the first completion request.
- **Complete Word** suggests nearby words from the same file when the language server has no matches.
- Accepting a language-server suggestion also inserts its imports in the same undo step.
- **Format Document** can format Go and JSON with no plugins installed.
- Enable **Prettier Formatter** for JS, TS, JSX, TSX, Vue, HTML, styles, JSON, YAML and Markdown.
- Enable **Ruff Formatter** to format Python files and `.pyi` stubs with your project's settings.
- Enable **ClangFormat Formatter** to format C and C++ sources and headers with `.clang-format`.
- **Settings → Plugins → Format on Save** turns formatting on for all supported languages.
- Choose Prettier's line width, quotes and semicolons for files without a Prettier config.
- Override Ruff's line length and quote style or choose a fallback style for clang-format.
- Set each language server's executable, TypeScript's log level and clangd's build and header options.
- Reset individual fields in **Settings → Plugins** to restore their defaults.

- **Conversation details → Export as Markdown** saves the current chat’s questions and replies.
- Large JSONL session logs open in read-only pages, including records with very long lines.
- Chat attachments use smaller, equal-sized chips and a paperclip button.
- **Sessions → Resume** opens the conversation in an editor chat, ready for your next message.
- Hovering over a session action keeps its button and tooltip steady.
- Choose Copilot’s Agent and model in **Settings → Agent → Copilot**.
- **Selection → Suggest Text with Copilot** offers an insertion for code or prose.
- Enable inline Copilot suggestions as you type. Press `Tab` to accept or `Esc` to dismiss.
- **Draft Commit Message with Copilot** fills the Git message box for review.
- Set `aiSecretFiles` to exclude additional files’ contents from Copilot commit drafts.
- Agent approval cards show the question first, with compact choices and expandable request details.
- Approval options that also apply to future requests appear in their own row.
- New approval requests scroll into view when you’re following the latest chat messages.
- Create a **Roundtable** with a topic and invite built-in roles, custom roles or Agents.
- Roundtable members speak under their own names and can hand the discussion to another member with `@`.
- Reopen a project’s roundtables with their members, saved messages and conversation settings.
- The speech bubble opens **Conversation details**, which stays open while you work in the chat.
- Drag **Conversation details** from anywhere on its title bar. It moves as soon as you drag.
- Agent startup and reconnection output appears directly in the chat as it arrives.
- **New Chat** groups Agent setup and plugin actions in the chat pane and wraps them in narrow splits.
- **Add File to Agent** shows a removable filename above the draft and sends its contents with it.
- Attach files to an Agent draft with the rounded file-plus button, drag and drop, or paste.
- Agent drafts show image thumbnails. Hover over an attachment to preview the image or file contents.
- Move into an Agent attachment preview to scroll through text or a tall image.
- **Settings → Plugins** searches installed and available plugins by name, category and install status.
- **Install Dev Plugin** links a local plugin folder and reloads its changes as you work.
- **Conversation details** shows the current repository’s added and removed lines and opens its diff.
- **Sources** lists files referenced in this chat and lets you add files and local image references.
- **Quick Open** and **Command Palette** share one untitled prompt for files, commands, symbols and lines.
- Branch, worktree, project and other pickers open in the same place as the **Command Palette**.
- Choose from eleven built-in roles in **Roundtable → Invite members**.
- Invite a Requirements Analyst, Interaction Designer, Compatibility Maintainer or Roundtable Moderator.
- **Settings → Agent → Role List** lets you add named roles alongside the presets.
- **Copy Role** creates an independent copy with the source role’s prompt, Agent and model.
- **Settings → Agent → Role List** imports and exports role definitions as JSON.
- Agents can create roles in batches with `ff roles import` or the local JSON API.
- Restore one role’s Agent, model and prompt in one click, or restore all presets together.
- Each role previews its prompt in the list, and each preset has its own checks and deliverables.
- **Load models** lists the selected ACP Agent’s models without sending a chat message.
- Terminal tabs, settings and the status bar use a rounded window icon with a `>_` prompt.
- **Maximize Panel** moves all terminals in this window to editor tabs; **Restore Panel** docks them.
- Use `+` to open another terminal, or **Split Right** to run two shells side by side.
- Split a docked terminal inside its panel or split an editor terminal in its workspace pane.
- **Close Terminal** ends only the selected session; closing the last one removes the panel.
- Restoring a window with saved terminal tabs opens those tabs without starting an extra panel shell.
- Terminals use the built-in shell or SSH; FiveLeaf no longer installs or connects to `herdr-core`.
- Opening the terminal panel immediately after a file sends typing to the terminal.
- **Search** and **Source Control** each remember whether files appear as a flat list or a folder tree.
- In **Source Control**, the dropdown beside **Fetch** is labeled **Remote Actions**.
- Changes lists filenames before their folders, with Git status and nonzero line counts.
- **Collapse All** folds every file in the current diff and becomes **Expand All**.
- The diff toolbar keeps change navigation in view and groups other tools under **More Diff Actions**.
- Short marks at the left edge of each chat show which turns are in view as you scroll.
- Hover over a mark to fan out nearby turns and preview the exchange. Click it to jump to the question.
- **View → Return to Current Chat** reopens your last focused chat.
- The sidebar activity buttons now line up with the workspace tabs.
- Chat no longer has an Agent list pane; both Agent menus show each target's connection status.
- Choosing an Agent still opens its chat if its status changes while the menu is open.
- Edit the current chat’s prompt and context limit in **Settings → Agent → Conversation settings**.
- Drag a tab out or choose **Move Tab to New Window** to see just its content, without the sidebar.
- Drag its title back to the tab bar, or choose **File → Move Back to Main Window**.
- Click **Split Right** to split as before, or hover to split left, right, up, or down.
- The editor's context menu adds selected text to Agent, or the whole file when nothing is selected.
- The arrow beside **New Chat** and the picker in the chat input list only registered Agents.
- Start a **New Chat** or open the project’s **Roundtable** above the session search field.
- A badge on **Sessions** shows how many Agent permission requests need a decision.
- Session search spans the sidebar, with two equal-width project filters below it.
- Opening a chat connects its ACP agent without sending the draft or imported history.
- A solid badge behind the Agent icon marks the chat that receives code selections, even across splits.
- Chat tabs, Agent menus and the composer use the robot icon when an Agent has no brand icon.
- Agent menus show brand icons on the left and color-coded status icons on the right; hover for details.
- The `+` for **New Chat** is the first button in the chat toolbar.
- Your chat messages appear on the right and replies on the left, without sender headings.
- Right-click **Sessions** in the sidebar to switch between open chat tabs.
- Chat tabs take their title from the first question you send.
- Agent tabs show a spinner while running and an icon for approval, completion, failure, or no task.
- Agent task updates appear in a popup with **View chat** to open the conversation.
- Agent status changes send desktop notifications you can click to return to the chat.
- Agent, Settings and plugin notifications use the current theme and stack in the lower right.
- Plugins can show a status icon, description and action button with `window.notify()`.
- The status bar bell stays visible and shows a dot for up to 100 unread notifications across restarts.
- Use the circled × to remove one notification, or choose **Clear** in the bell panel to remove them all.
- **New Chat** and **Split Right** open independent chats, even with the same agent.
- Chats open only in workspace tabs; the sidebar still lists your session history.
- Codex agents prefer the installed Codex CLI and retain the adapter's bundled fallback.
- Thinking choices work immediately after connecting and preserve an Agent's provider default.
- Agent's model picker stays open after a choice and opens above the shorter draft.
- The model picker leaves the context usage bar visible and clickable.
- The thinking slider shows just the selected level at its right edge.
- Thinking intensity uses English levels and the levels the current model advertises.
- The thinking slider shows a hand on hover, follows your drag, and snaps to a level on release.
- Changing thinking intensity no longer flashes the chat or shows a reconnecting state.
- Agent's model and thinking pickers fade in more quickly and flow more smoothly.

- **Sessions** removes duplicate records and Codex helper conversations from the main list.
- **Sessions** includes Pi conversations and filters the current project's history by agent.
- **Continue with…** carries conversation history into a separate chat with your chosen agent.

- Agent keeps a compact input at the bottom, with context usage drawn along its top border.
- Agent's model picker adds search, server groups, recent choices and keyboard navigation.
- The compact Agent model button shows only the model name.
- Choose thinking intensity inside the model picker, with level colors and flowing highlights.
- Wide Agent pages show the workspace directory beside the model controls.
- Agent's context bar shows capacity and a Pi compaction marker; click it to view usage details.
- Agent's context bar shows the used percentage on the left and capacity on the right.
- The whole context bar shows a hand pointer, from the percentage to the capacity.
- Context details show exact token counts and separate free space from auto-compaction reserves.

- Agent replies render lists, tables, quotes and code blocks with theme-matched syntax highlighting.
- Agent code blocks show their language, scroll long lines and offer **Copy code**.
- Agent keeps tool calls in order and lets you expand thinking and tool details.
- Agent adds **Copy Message** and **Jump to Latest** while preserving your reading position.

- One Dark Pro colors Go variables red, types yellow and built-in types purple, with italic parameters.
- One Dark Pro uses a quieter gray for selected sidebar rows and controls.

- Agent's **Model** menu changes the conversation model and keeps the choice across new chats.
- Agent shows reported context usage in a bar, with percentages and token counts in its details.

- **Settings → Agent** has its own page for Agent connections and roles.
- **Registered Agents** lists custom and plugin agents together, including existing Pi registrations.

- The compact chat composer keeps its model, agent and send controls without an add-context button.
- Agent page switches keep drafts, replies and pending permissions, including while work is running.

- Hover over code to read documentation and diagnostics beside the symbol without leaving the editor.
- Hold `Cmd/Ctrl` over a name to underline it; clicking jumps without changing the source selection.
- Definition jumps ignore blank space and offer a picker when a symbol has several destinations.
- Hover cards dismiss on edits and scrolling, and slow replies no longer reopen dismissed cards.

- Settings and Roundtable blend window controls into the page, with one background color.
- The project rail hides at the window's left edge and slides out when the pointer reaches it.
- `gd` goes to a definition in Vim and Helix modes, as `F12` does.
- `⌘`-clicking a name in the source goes to its definition.
- The sidebar is an opaque, darker rounded card, and the gap around it matches the window.
- Editor line numbers stay as translucent as the code in a transparent window.
- The title bar, tabs and status bar share the editor's color for one surface.
- The sidebar starts on the left, with the terminal button at the bottom right.
- Source Control keeps Fetch and the repository menu on its top row, with no separate branch row.
- Source Control's commit message stays at the bottom, with the commit button inside it.
- Incoming and outgoing commit counts move from the title bar into **Source Control → Fetch**.
- **Switch Branch…** lists a remote branch even when a local branch has its name.
- Diffs in one column show one column of line numbers, not the old side's and the new side's.
- Git sync leaves the status bar; fetch, pull and push stay in Source Control and the Git menu.
- The language server control leaves the status bar out until a server starts in the workspace.
- Settings saves can retry after disk errors without losing preferences or window state.
- Invalid saved window or session entries no longer prevent the app from starting.
- Failed folder setting changes keep the previous settings.
- Cancelling language-server requests stays responsive when the server stops reading.
- Saving through a symbolic link updates its target and keeps the link.
- Copying a folder through linked paths no longer recursively copies it into itself.
- `ff plugin pack` can run again and preserves the previous files when packing fails.
- `ff plugin pack` reports backup files it cannot remove after packing.

- Invite previously saved named roles to roundtables.
- Roles apply their model before sending and keep the draft when model selection fails.
- Agent configuration import and export includes roles and still accepts older files.

- Markdown's **Edit** button opens live editing with a compact formatting toolbar.
- **Edit → Markdown** adds bold, italic, strikethrough, code, links, images and table insertion.
- Clicking live Markdown list text places the caret without selecting the next character.

- Project tabs have larger tiles, clearer selection and a simpler add-project button.

- Switching chats keeps drafts, replies and the sidebar's session search in place.
- Session titles and details stay visible when selected or hovered in One Dark Pro.

- **View → Tasks** searches and runs tasks, with built-in `lazygit` and `yazi` entries.
- `⌥⌘T` on macOS or `Ctrl+Shift+B` elsewhere opens the task picker.
- Global and project `tasks.json` files add commands, arguments, working directories and environment.
- Tasks reuse running terminals and keep output and exit codes, with **Rerun Last Task**.

- Sidebars have more space around their rounded surfaces while keeping activity icons centered.
- One Dark Pro gives the sidebar a darker inset surface with rounded corners.
- Editor tabs flow into the path row without a separator line.
- The title and status bars blend into the window without separator lines.
- Sidebars use inset surfaces and centered activity buttons, with scrolling for extra activities.
- Tabs use quieter selected styling and align with the path below them.
- The title bar keeps project controls without repeating the file name.
- Vim and Helix modes move to the left of the status bar.
- One Dark Pro uses consistent borders and selected controls throughout the workbench.
- Search fields and settings groups share the same control colors as the rest of the interface.

- Catppuccin file icons now offer Latte and Mocha; Frappé and Macchiato are removed.
- Switching agents or roles keeps each conversation's own draft and history.
- **Roundtable** opens a separate window with agent names, timestamps and reply hop counts.
- Roundtables route by leading `@name`, capability tags or round-robin, with at most three replies.
- Chat settings add system prompts and recent-message limits; new roundtable messages interrupt replies.
- Custom agents support CLI commands, working directories, capability tags and reply timeouts.
- Agent settings add enable switches and JSON configuration import and export.

- The title bar gently emphasizes the project name without a folder icon or following slash.
- The top search box is removed; `Cmd/Ctrl+P` still opens Quick Open.
- JavaScript and TypeScript distinguish functions, properties, parameters and template expressions.
- One Dark Pro adds italic comments and parameters, with lighter title, tab and status bars.
- Quick Open, command and symbol searches use inset fields with clearer focus borders.
- Settings search and action buttons have more consistent spacing and rounded surfaces.

- **Settings → Appearance → Mica Material** imports and exports material settings as JSON presets.
- One Dark Pro aligns editor highlights, Git marks and terminal colors with the VS Code theme.
- The title bar now shows the project, worktree and branch; click each to search and switch.
- The project picker offers recent projects and **Open Local Folder…** or **Open Remote Folder…**.
- Recent project rows can open in the current window or a separate window.
- Hover a recent project to reveal it in the file manager or remove it from the list.
- Removing a recent project keeps its files and saved session.
- The main worktree is labelled `main`, with its actual branch shown separately.
- **Switch Worktree… → Create Worktree…** creates a checkout from a new branch name.
- Worktree and branch pickers keep selected names visible with a clear outline as you press `↑` or `↓`.

- `F12` and `Cmd/Ctrl`-click navigate from a diff symbol to its definition in current code.
- Diff context menus offer **Go to Definition** and **Show Hover** through language servers.
- **Configure Agents → Add Custom Agent** registers ACP programs without editing JSON.
- Custom Agent settings support editing, separate arguments and environment values, and removal undo.
- Optional ACP plugins add Gemini CLI, Qwen Code, OpenCode and Kimi Code.

- **Settings → Theme** groups color and file icon themes; **View** groups layout controls.
- **Settings → Theme → File Icon Theme…** includes Ayu file and folder icons.
- The top-right **Settings** menu adds **Open Settings (JSON)** and puts **Command Palette…** first.

- ACP chats show streamed replies, tool activity and permission choices.
- Enable the Codex or Claude ACP plugin, or configure another adapter with `agentServers`.
- `⌥⌘L` / `Ctrl+Alt+L` adds selected code to the Agent draft; file menus add whole files.
- **Add Review to Agent** puts code review comments and their source into the chat draft.
- Agent file changes open as before-and-after diffs alongside **Review Workspace Changes**.
- Agent reads include unsaved editor text, and writes preserve files with unsaved changes.

- Theme and icon pickers show the previewed row with a clear outline and check as you press `↑` or `↓`.
- **View → Theme → Ayu** adds light and dark palettes that follow **Appearance**.

- **Settings → Appearance → Mica Material** adds a frosted background to any color theme.
- Mica's frost, hue, intensity and speed sliders preview changes while dragging.
- **Animated Mica Background** toggles flow; Reduce Motion also pauses it.

- Sidebar icons stay visible when selected, including in One Dark Pro.
- The selected sidebar view has an accent underline that stays visible after the pointer moves away.

- **Settings → Theme → File Icon Theme…** previews file icons as you press `↑` and `↓`.
- In the icon picker, `Enter` saves your choice and `Esc` cancels the preview.
- One Dark Pro keeps selected list labels and icons visible.
- **Settings → Appearance → File Icons** previews and switches file and folder icon themes.
- File icons include Catppuccin Latte and Mocha, Material Icon Theme and Seti.
- Plugins can install additional icon themes, with default icons when a pack is unavailable.
- **View → Theme** and appearance settings now include One Dark Pro.

- The top-right **Settings** menu includes color themes, shortcuts, plugins and layout controls.
- The welcome page removes duplicate sidebar, terminal and color theme actions.

- Language server plugins cover common frontend, backend and configuration files.
- `Space` shows a compact Leader menu in the bottom-right corner in Vim and Helix modes.
- Vim and Helix `Ctrl-w` keys split, focus, close and balance IDE editor columns.
- Bind `go.leaderMenu` to open the Leader menu with a personal shortcut.
- **Language Servers** is available from the command palette and the Leader menu.

- File breadcrumbs include the project folder, which opens its files when clicked.

- The title bar is slimmer, and sidebar navigation uses centered icons with hover labels.

- **Settings → Keyboard Shortcuts** lists installed keymaps alongside FiveLeaf’s default shortcuts.
- Plugins can add keymaps with chords, focus conditions and keys for each operating system.
- Personal shortcuts override the selected keymap; resetting a shortcut restores that keymap.

- A single code-and-lightning icon opens language server states from the status bar.
- Restart or stop one language server, or all servers in the current workspace, from the status bar.
- Vim `gs` and `gS` jump to labeled words and lines, keeping visual selections.
- Vim adds `:HopWord`, `:HopLine`, `:HopChar1` and `:HopChar2` for visible-text jumps.
- Helix `gw` shows two-letter labels and selects the chosen word.
- Right-click the filename below the tabs to copy its full path directly.
- Click the filename below the tabs to choose a symbol and jump to it.
- Starting a text selection on a distant line keeps the caret gliding with its trail.
- Settings groups fonts, Markdown and key modes under **Editor**, with tabs for each.
- **Version Control** brings Git, diffs and merges together in a simpler settings sidebar.
- Settings controls stack below their descriptions in narrow windows.
- File tabs are more compact, with rounded corners, spacing and a softly raised active tab.
- Sidebar and terminal buttons now sit at the bottom, with independent left or right positions.
- Right-click either button for **Bottom Left** or **Bottom Right**; each choice stays after restart.
- Long keyboard jumps keep smooth caret movement and trails in source and Live Preview.
- Keyboard selection in Live Preview keeps smooth caret movement and trails.
- Vim and Helix `h/j/k/l` moves with counts keep their smooth caret movement and trail.
- **View → Line Numbers** switches between absolute and relative line numbers and remembers the choice.
- The app is now **FiveLeaf**, with the `ff` command and `fiveleaf://` links.
- Existing Mayoi plugins and folder settings remain supported.
- Automatic updates stay off until a FiveLeaf release source is configured.
- **Help → Report a Problem…** copies the report when no issue tracker is configured.
- Packaged apps now include the original MIT license and copyright notice.

- Themes can live in `~/.fiveleaf/themes`, with colors, code, terminal, layout and type sizes.
- Themes can also be written under `themes` in `settings.json`, by id.
- **View → Theme** lists those themes after FiveLeaf's own.
- **Plugins ▸ Open Themes Folder** opens that folder, with an example theme the first time.
- `ff plugin new <name> --theme` writes a theme plugin to start from.
- The app icon is now a hand-drawn four-leaf clover, in light and dark.
- **Settings ▸ Appearance** adjusts background opacity over frosted glass while keeping text clear.
- The caret glides to its new place as Neovide's does; **Settings ▸ Editor ▸ Smooth Caret** turns it off.
- Smooth caret movement leaves a connected trail that bends and catches up when the caret stops.
- Helix normal mode now shows smooth caret movement and trails in source and Live Preview.
- Caret trails are brighter and linger longer as they catch up.
- Mouse clicks glide between distant lines, including clicks into Live Preview blocks.
- **View → Helix Keys** edits with the core of Helix's keys, alongside **View → Vim Mode**.
- Signing in and syncing settings are gone; settings stay in `settings.json` on each machine.
- The welcome page no longer asks you to star the project on GitHub.
- The window's dark themes separate the editor, the panel and the chrome into clearer layers.
- Settings switches and segmented controls now look and move like the rest of the window.
- Tabs, breadcrumbs, the status bar and the sidebar now share one set of gaps, radii and text sizes.
- The sidebar's width and whether it's shown now stay the same as you switch projects and windows.
- **Sessions** keeps **All Projects** chosen as you switch projects.
- **Sessions** marks a session that's open in a terminal in any project with a terminal icon.
- Clicking such a session switches to its project and shows that terminal.
- Opening local files keeps the window responsive while their tabs show loading progress.
- A file on a network share that stops answering no longer keeps other files from opening.
- Files that fail to open keep their tabs, error details and a **Try Again** action.
- Quick Open stays responsive while checking file paths, including unavailable network shares.
- Filtering Quick Open, symbols or commands keeps the selected result in view.
- A folder holding many Git repositories no longer makes the window sluggish.
- Typing a bracket or quote that closes itself no longer scrolls or shakes the editor.
- In Markdown, `Enter` continues a list, task list or quote, and on an empty item ends it.
- In Markdown, `⌘Enter` switches a line between a list item, an empty task and a checked task.
- **Settings ▸ Shortcuts** lists `⌘Enter` for **Toggle Task**, not **Insert Line Below**.
- In Markdown, `⇧⌘Enter` opens a new item of the list or quote above.
- In Markdown, `Tab` and `⇧Tab` indent or outdent a whole list item wherever the caret is.
- **Edit ▸ Markdown** toggles tasks, lists, quotes, headings and embeds, and clears a line's formatting.
- `⌥⌘1` to `⌥⌘6` toggle a Markdown heading of that level on macOS.
- **Selection ▸ Join Lines** joins the selected lines, or the caret's line with the next, into one.
- **Sessions** shows what Qoder and Codex sessions do, as it does for Claude Code sessions.
- **Sessions** shows what agents are doing in FiveLeaf's terminals.
- New Live Preview edits Markdown as it reads: headings, emphasis, links and code styled, their marks hidden.
- In Live Preview, the marks of what the caret touches show, faint, fading in as the caret reaches their line.
- Live Preview draws bullets, task boxes, quotes, alerts, pictures and math in place, as the Reading view does.
- Live Preview renders tables, Mermaid diagrams and display math, and shows their source when the caret goes in.
- In Live Preview, text below a table, diagram or formula glides as its source shows or hides.
- Bullets, task boxes and list indents in Live Preview keep their width as the caret reaches them.
- Nested lists and lines that continue a list item stand under the item's text in Live Preview.
- Quotes in Live Preview have one unbroken bar, and the largest headings a hairline below, as in Reading.
- A click on a task's box in Live Preview checks it, filling the box and striking its text through.
- A click on a task's box in the Reading view checks it in the file.
- A picture from the web in Live Preview shows a card in its place while it loads, then fades in.
- Symbols such as `⌘` and wide characters keep a table's columns lined up in its Live Preview source.
- Headings in Live Preview have room above them, and their lines stay put as the caret reaches them.
- Footnote definitions like `[^1]: text` show as their number in Live Preview until the caret reaches them.
- Links to web pages in Reading view end with the same external-link icon as in Live Preview.
- Embedded notes like `![[note]]` now show in place inside list items and quotes, in both views.
- Holding `⌘` over a link in Live Preview shows a pointing hand at once, and letting go hides it.
- Buttons at the right of a Markdown tab's path switch it between source, Live Preview and the Reading view.
- Switching between source, Live Preview and the Reading view fades the new view in.
- **View → Toggle Live Preview** (`⌥⌘V`) switches a Markdown tab between Live Preview and its source.
- **View → Open Markdown As** chooses source, Live Preview or the Reading view for files you open.
- Live Preview stays smooth as you type and scroll in long notes full of tables, math and diagrams.
- Live Preview shows `#tags` as colored pills, as in Obsidian.
- Live Preview raises footnote references like `[^1]` as small superscript numbers, as Reading view does.
- Live Preview hides the backslash of escaped characters like `\*` until the caret reaches them.
- Links in Live Preview are underlined, and wikilinks to notes that don't exist yet look faded.
- In Live Preview, a link's underline fills in under the pointer; `⌘`-click it to open it.
- Callouts like `> [!tip] My title` show their own title, and Obsidian's callout types are recognized.
- Callouts are tinted in their color in both Live Preview and Reading view.
- Each Obsidian callout type, like `[!bug]` or `[!example]`, has its own color and icon.
- Callouts marked `[!faq]-` start folded and `[!faq]+` start open; click the chevron to fold or open them.
- Embedded notes like `![[note]]` or `![[note#Heading]]` show their content in place, in both views.
- Click the name above an embedded note to open it.
- `%%comments%%` look faint in Live Preview and are hidden in Reading view.
- Links to web pages in Live Preview end with a small external-link icon.
- Reading view shows `#tags` as pills, as Live Preview does.
- Links in Reading view are faintly underlined, and wikilinks to missing notes look faded, as in Live Preview.
- A folded callout stays folded when you edit its title.
- Block embeds like `![[note#^id]]` show that paragraph, list item or block in place.
- Block ids like `^id` look faint in Live Preview and are hidden in Reading view.

## Mayoi release history

### 0.20.1

- In **Sessions**, `⌘`-click, `Shift`-click, `Shift`+arrows and `⌘A` select several sessions.
- **Copy Session ID** on one of several selected sessions copies all their IDs, one per line.
- Drag one of several selected sessions to open all their transcripts, or type their paths in a terminal.
- A tab dragged onto another project's square stays under the pointer as that project shows, to drop in a column.
- **View → Attach to Terminal in herdr-core…** shows one of its terminals in a terminal here, attached.
- Closing such a terminal, or `Ctrl+\`, detaches it, and the shell in herdr-core runs on.
- `mayoi run view.attachHC=<address>` shows a terminal of herdr-core by its address, for plugins too.
- On macOS, VoiceOver no longer crashes Mayoi on text that starts with a byte order mark.
- Clicking a Git diff's toolbar buttons while its changes refresh no longer crashes Mayoi.
- On a Mac with Apple silicon, the Intel build of Mayoi updates to the Apple silicon build.
- The Intel build on Apple silicon says at launch that it runs slower and can crash, with a **Download** button.

- Right-click a session in **Sessions** to copy its transcript file, open it in the editor, or reveal it.
- Drag a session from **Sessions** to the editor to open its transcript, or to a terminal to type its path.
- A resumed session's tab keeps the session's title in a shell other than your default one.
- On Linux and Windows, `Page Up` in the **Sessions** list goes back up to the first session.
- Opening a file in Mayoi while it is still starting no longer crashes; the file opens once it's ready.
- On Linux, letters no longer crowd together or leave gaps in fonts with kerning, such as Microsoft YaHei UI.
- **Settings → Editor → Tab Size** sets how many columns a tab takes, 4 by default.
- Tabs now take the same width on every system, where Linux showed them 8 columns wide.

### 0.20.0

- Plugins can add coding agents with `contributes.agents`, whose sessions show in **Sessions** with their own icon.
- A plugin agent's sessions come from files a glob matches, or from a function registered with `agents.register`.
- A plugin agent's session resumes in an editor tab with its `resume` command, as the built-in agents' do.
- A slow or failing plugin agent never holds up **Sessions**, and its error shows in **Plugins → Show Plugin Log**.
- Turning a plugin off fades its agent's sessions out of **Sessions**.
- A resumed session's tab is titled after the session until the agent names it.
- Terminal tabs kept in herdr-core keep their agent's icon across a restart.
- Resuming a session in **Sessions** opens it in a tab of the editor column you were in, ready to type.
- A session already open in a terminal shows that tab again instead of starting another.
- Each **Sessions** row shows an icon for its agent: Claude Code, Codex, Qoder or Grok.
- A terminal tab running a coding agent shows that agent's icon.
- In **Sessions**, `Page Up` and `Page Down` move the chosen session by a page, from the search field or the list.
- `Home` and `End` in the **Sessions** search field choose the first or last session (`Ctrl` on Windows and Linux).
- The chosen session glides to its new row and keeps the next row in view as the keys move it.
- The chosen session is ringed while the **Sessions** list has the keyboard, and dims when the keyboard is elsewhere.
- A session that drops out of **Sessions** fades out faster, and one that comes back mid-fade picks up where it was.
- Clicking a repository's name or row background in source control expands or collapses its changes.
- **Settings > Shortcuts** lists **Report a Problem** under Help instead of Account.
- A `mayoi://file` link opens its file at the line and column it names.
- On Linux, Mayoi is offered for text, Markdown, JSON and other text files, and `xdg-mime default` takes it.
- On Linux, opening a file in Mayoi from the file manager opens that file.
- On Windows, **Open with** lists Mayoi for text, Markdown, JSON and other text files.
- LocatorJS's **Custom link** reaches Mayoi with `mayoi://file/${projectPath}${filePath}:${line}:${column}`.
- A `mayoi://` link to a missing file says Mayoi could not open it, in the window in front.
- Math with `\tag{…}` now renders in the Markdown preview, its label beside the equation.
- After a full-screen program is killed, scrolling or typing in the terminal no longer prints escape codes.
- The sidebar's new **Sessions** view lists Claude Code, Qoder, Codex and Grok sessions, the latest first.
- Sessions show for **This Project** or **All Projects**, and the search field narrows them by title, folder or agent.
- Enter or a double click resumes a session in a new terminal in its folder.
- **Sessions** groups sessions under Today, Yesterday, Previous 7 Days and Older, each with its count.
- Each session row shows its title on one line, with its agent, folder and last use under it.
- While **Sessions** reads for the first time, it shows placeholder rows, which the sessions then replace.
- Claude Code sessions of every `~/.claude-*` folder show too; `agentSessionsClaudeDirs` adds more folders.
- With `terminalBackend` set to `"hc"`, a running session's dot and words say whether it works or waits for you.
- Resuming a session that runs in an hc terminal shows that terminal instead of starting another.
- In **Sessions**, `↑` and `↓` in the search field move the chosen session without leaving the field.
- The chosen session stays chosen when sessions are read again, and sessions that appear fade in.
- Hovering a session shows buttons to resume it or copy its resume command.
- In **Sessions**, the search words are marked in each session's title, agent and folder.
- With the keyboard in the sessions list, the chosen session shows its resume and copy buttons.
- `⌘C` in the sessions list copies the chosen session's resume command.
- The **This Project** / **All Projects** switch in **Sessions** no longer clips in Japanese.
- When a search matches a session's first prompt, folder or id, its row shows that text under the title.
- With `terminalBackend` set to `"hc"`, a working session's dot breathes, and one that starts waiting pulses.
- **Sessions** rows sit slightly apart, and a hovered row's background fades in and out with its buttons.
- Sessions that appear as **Sessions** is read again open their place, and the rows under them slide down.
- Sessions that disappear on a re-read close their place as they fade, and the rows under them slide up.
- Typing in the **Sessions** search field stays quick with thousands of sessions.
- When a session starts or stops running, its state slides in or out and the agent beside it glides over.

### 0.19.3

- Language server plugins can run a `resolve` command that sets the server's arguments, environment and settings.
- Language server plugins can map paths so definitions and diagnostics keep the workspace's symlinked paths.
- Mayoi no longer waits on a language server that has already stopped.
- The command palette, Quick Open and pickers show at once when a key opens them, and fade in on a click.
- Animations share a few lengths, so what moves alike now moves alike.
- Choosing a tab no longer makes its title bolder and shifts the tabs after it.
- The caret's line and column in the status bar, and find's match count, no longer jiggle as digits change.
- Remote SSH and Mermaid commands show their menu in the command palette instead of a bare `:`.
- A terminal whose shell exits as it starts stays open with what it printed, and offers **Report…**.
- **Help → Report a Problem…** opens a new GitHub issue with Mayoi's version and your system filled in.
- Inline blame, commit tips and settings sync show a date for anything older than a week.
- The merge view's count of resolved files reads "1 of 1 file resolved" in English.
- Inline blame now shows with **View → Word Wrap** on, after the caret line's last row.
- When a wrapped line's last row is too full, its inline blame is cut short with `…`.
- Dragging over code in a diff selects text across lines; dragging on the line numbers still selects rows.
- Double-clicking a word in a diff selects it, and a triple click selects the line; `E` opens the file.
- `Cmd+C` in a diff copies the selected code with its indent, without line numbers or `+`/`-` marks.
- Side by side, a selection stays on the side where it started.
- `Cmd+F` in a diff tab finds in the lines it shows on both sides, in every file, with a match count.
- `Enter` and `Shift+Enter` in a diff's Find go to each match, unfolding a collapsed file to show it.
- **Git → Collapse All Files in Diff** and **Expand All Files in Diff** fold the current diff's files.
- On macOS, `Option+Cmd+[` collapses all diff files and `Option+Cmd+]` expands them.
- `mayoi settings` reads and changes preferences, with Mayoi running or not.
- `mayoi settings --folder` in a project's subfolder changes the project's `.mayoi/settings.json`.
- Choosing **Overwrite** for a remote file that changed on its host no longer asks again while an autosave runs.
- After a remote connection is lost, the banner shows its first try to reconnect as attempt 1.
- A question that comes up in the background now waits instead of replacing the dialog you're answering.
- Questions that come up in the background, as a remote file's conflict, wait for an open picker to close.
- Pressing `Cmd+Q` or a window's close button again while the unsaved-changes question shows brings it forward.
- Saving a remote file while another save is under way no longer loses a pending save and close.
- **Revert** or `Escape` on the "changed on host" question keeps the tab open instead of dropping the save.
- Autosave no longer writes to the host while the "changed on host" question waits for an answer.
- With format on save, saving twice in a row waits for the formatter instead of writing unformatted text.
- Chained shortcuts from `runCommands` wait for Git commands in a selected submodule or nested repository.
- During a merge, rebase or cherry-pick, branch switching, stash, fetch, pull and push stay disabled.
- The mascot hops for Git commands in the selected repository, not only the folder's own.
- After switching folders, **Quick Open** lists the new folder's files without waiting for the old folder's listing.
- Files Git ignores show faint right after switching folders, without waiting for the old folder's read.
- Submodules and nested repositories are still found when **Initialize Repository** runs during a scan for them.
- **Running Plugins** and the **Plugins** settings page agree on the state of a plugin that is off or can't load.
- A language server that failed to start tries again when you open a file of its language 30 seconds later.
- Reloading, restarting or turning on a plugin starts again its language servers that failed to start.
- A language server that fails to start shows its error once, when it fails, instead of on every completion.

### 0.19.2

- With `terminalBackend` `"hc"`, terminals show up in the Herdr app, in a space for each folder.
- Terminals already running in herdr-core show up in Herdr too once Mayoi attaches them again.
- Set `terminalBackendHcShowInHerdr` to `false` to keep new terminals out of the Herdr app.
- Overwriting an SSH file that changed on its host no longer swaps the editor back to the host's old text.

### 0.19.1

- **Word Count** and **TODOs** come with Mayoi but start off; turn them on in **Settings → Plugins**.
- A plugin's `plugin.json` can say `"enabled": false` to start off until you turn it on.

### 0.19.0

- **Plugins → Show Running Plugins** opens a live table of each plugin's state, CPU time, memory and crashes.
- Click a column head in **Running Plugins** to sort by it; values refresh every second while the tab shows.
- Each row of **Running Plugins** can restart its plugin, turn it off, open its details, or show its log.
- **Plugins → Show Plugin Log** opens a live log that follows new lines, with a **Jump to latest** pill.
- Filter the **Plugin Log** by plugin, level and text, and use **Copy** and **Clear** in its bar.
- The **Plugin Log** keeps the last 10,000 lines and stays fast however long it gets.
- A plugin view's open folders and selection are now kept per window.
- Two lists in the same plugin view no longer show the same row as selected.
- Built-in status bar items that do nothing on click, such as the problem at the cursor, show their tooltip again.
- A plugin's settings now live only in its details; its row on **Settings → Plugins** links to them.
- Plugin sidebar views, such as **TODOs**, match the Explorer's rows, file icons, hover and selection.
- Plugin sidebar views show a loading state, a clear empty state, and fade in when their content arrives.
- Plugin status bar items, such as the word count, glide to a new width instead of shifting their neighbours.
- Plugin status bar items sink when pressed and show their tooltip even when they have no command.
- Plugin pick boxes look like the command palette, with its key hints and a shake when nothing matches.
- The command palette, pickers and prompts fade and scale in as they open, unless Reduce Motion is on.
- Click a plugin on the **Settings → Plugins** page to open its details; `Esc` goes back to the list.
- A plugin's details list its commands with their shortcuts, menus, status items, views, themes and more.
- A plugin's details show its memory, CPU time and start time, refreshed every second while open.
- A plugin's details show its recent log lines and its settings, with **Open Full Log** for the whole log.
- Plugin errors now show on the Plugins page as soon as they happen.
- The disabled **Install** button on the Plugins page now looks clearly disabled in dark themes.
- `mayoi plugin new` fills in `author` from your Git user name.
- **Find in Files** (`Shift+Cmd+F`) selects the previous query, so typing or pasting replaces it.
- Drag a file's tab onto a project in the rail to move it there; hold `Option` to copy it instead.
- Rest a dragged tab on a project in the rail to switch to it, then drop the tab where you want it.
- Unsaved changes come along when a tab moves to another project.
- Right-click a tab and choose **Move to Project** to send it to another project.
- New plugins: folders of JavaScript or TypeScript in `~/.mayoi/plugins`, which Mayoi runs with no Node or build step.
- Plugins add commands, shortcuts, menu items, status bar items, sidebar views, previews, themes, languages and snippets.
- `mayoi plugin install <git-url>` installs a plugin, and `list`, `update` and `remove` manage them.
- `mayoi plugin new <name>` writes a new plugin, and `mayoi plugin dev` reloads it each time you save.
- `mayoi plugin pack` copies the npm packages a plugin imports into it.
- The **Plugins** menu, and the **Plugins** page of the settings window, install, update, remove and configure plugins.
- **Plugins → Show Plugin Log** and **Show Running Plugins** show what plugins logged and how much they use.
- A plugin that loops or keeps too much memory is stopped, and `mayoi --disable-plugins` turns plugins off.
- The **Plugins** settings page shows each plugin's version, author, source, and a colored dot for its state.
- The **Plugins** page asks before it removes a plugin, and shows install and update errors on the plugin's row.
- While the plugin index loads, the **Plugins** page shows placeholder rows, and **Try Again** if it fails.
- Switches in the settings window respond to hover and stretch while held.
- A plugin's `plugin.json` and its index entry can name an `author`.
- The **Plugins** page shows no error while the plugin index isn't published yet.
- **Selection → Format Document** (⇧⌥F) formats with a plugin, and the `formatOnSave` setting formats as you save.
- Plugins underline problems in the editor; the status bar counts them, and **Show Problems** lists them.
- Plugins can bring language servers, for **Go to Definition** (F12), **Show Hover** and **Complete Word** (⌃Space).
- Plugins can add themes, including VS Code color themes as they are.
- **Selection → Insert Snippet…** inserts snippets that plugins bring, in VS Code's format.
- Each function a `.ts` file in `~/.mayoi/scripts` exports is a command; **Run Selection as Script** runs code once.
- Signed in, the plugins you installed from Git follow you to your other machines.
- New **Word Count** plugin shows the words of a Markdown file and its reading time in the status bar.
- New **TODOs** plugin lists the folder's TODO, FIXME and HACK comments in the sidebar.

### 0.18.2

- On Windows, right-click menus no longer cut off the end of their labels, as in Chinese.
- The Markdown preview highlights `==text==` and `<mark>` text with a yellow background.
- In Markdown, typing `=`, `*`, `_`, `~` or `$` over a selection wraps it; type it twice for `==` or `**`.
- The Markdown editor highlights `==text==` with a yellow background, as Obsidian does.
- The Markdown editor colors bold, italic and struck text between Chinese characters, as in `会**整体**线`.
- The Markdown editor shows `**bold**` in bold and draws a line through `~~struck~~` text.

### 0.18.1

- Opening a folder you switched away from no longer shows the terminals another window kept from it.
- On Windows, lines starting with tabs, like Go code, keep their indent, at four columns a tab.
- On macOS, Mayoi appears in Finder's **Open With** menu for text, Markdown and source code files.
- AI commit messages leave out the lines of more secret files, like `.envrc`, `*.tfvars` and `~/.aws` files.
- A changed `.env` or key file that is already committed no longer has its lines sent to the AI either.
- Keys, tokens and passwords in any changed line are swapped for `[left out]` before the diff goes to the AI.
- New `aiSecretFiles` setting adds your own patterns of files whose lines AI commit messages leave out.

### 0.18.0

- AI commit messages no longer send the contents of new secret files like `.env` or keys, only their names.
- With the `"hc"` backend, `terminalBackendHcPath` finds hc before `PATH` and `~/.local/bin`.
- With the `"hc"` backend, `terminalBackendHcEnv` passes `KEY=value` variables to hc's commands.
- With `terminalBackendHcPath` set, Mayoi no longer offers to install hc.
- `terminalBackendHcPath` and `terminalBackendHcEnv` stay on each machine and never upload with settings sync.
- Rename a terminal from its tab's right-click menu or **View: Rename Terminal…** in the command palette.
- A renamed terminal keeps its name when programs set a title; rename it to nothing to show theirs again.
- Pressing Enter in the crash report question chooses **Don't Send** instead of sending the report.
- On Windows, files indented with tabs, like Go code, show their indentation again.
- Find's match count changes instantly instead of rolling, in the editor, preview and terminal.
- The Git commit list and project rail use less CPU redrawing author and project initials.
- Windows can install `mayoi.cmd` from **File → Install 'mayoi' Command**, including user PATH setup.
- The host box of **Connect to Host…** has an **SSH Config** button that opens `~/.ssh/config` in a tab.
- **File → Remote SSH → Open SSH Config** and the command palette open it too, creating it when missing.
- Typing in Markdown with blank lines between paragraphs takes less CPU to keep the colors up to date.
- Terminals in tabs you can't see no longer redraw the window as they print, which saves CPU.
- Typing an absolute or `~/` path in Quick Open offers that file, even with no folder open.
- A path in Quick Open can end in `:line` or `:line:column`, and opens the file at that spot.
- Quoted or backslash-escaped paths pasted from a terminal open in Quick Open too.
- A project whose program works shows a turning ring at the corner of its square on the rail.
- When work finishes or fails while other work goes on, its dot shows inside the ring.
- The rail marks a project amber while it waits for you, green when done and red when work failed.
- A program waiting for you keeps its project's mark until it stops waiting, even after you look.
- Hovering a project on the rail lists what its programs wait for and work at, with their progress.
- The work ring on a rail square turns smoothly and fills to show progress when programs report it.
- A rail square's ring and dot grow in and fade out, and the dot blends from one state's color to the next.
- The rail tip marks waiting, finished and failed work with icons and shows progress as a small bar.
- A rail square's progress ring keeps its fill as it fades out, and completes when the work finishes.
- A rail square's dot sends out one ring in its color when work finishes or fails.

### 0.17.0

- Terminal icons show whether a program says it works, waits for you, finished or failed (OSC 7501).
- While a program works or waits for you, a pill over its terminal says what it does, with its progress.
- A program in a project out of sight that waits for you, finishes or fails marks the project on the rail.
- Typing in a terminal clears the mark of a program's finished or failed work.
- The program status pill shows the program's name in bold, its message, and its progress as a bar.
- When a program waits for you, its pill gets a colored ring and its icon pulses in time with the badge.
- As shown work finishes or fails, the pill says so with a check or a cross for a moment.
- The pill counts the program's other active tasks, and the terminal icon's tooltip lists them.
- A program that reports its status very often no longer slows the window down.
- **Rename Active File…** in the command palette renames the current tab's file, bindable as `file.rename`.
- Opening a file from outside the folder clears the selection in the Explorer.
- Splitting a Markdown preview to the right keeps it a preview, scrolled to the same place.
- **New File…** and the other items of the Explorer's menu below the files no longer do nothing at times.
- A plain mouse wheel scrolls the editor tabs sideways when more of them are open than fit.

### 0.16.2

- **View → Theme → The Matrix** puts the code's green inside steel chrome, like the film's real world.
- The Matrix gives each kind of code its own hue, and shows comments in steel that reads at 6.9:1.
- Under The Matrix, primary buttons are the red pill, chosen rows the blue, and Discard is orange.
- The White Rabbit is The Matrix's character on the welcome page and in the status bar, hopping while Git runs.
- The Matrix's digital rain is mirrored, as in the film, with a dimmer far layer behind it.
- Quick open, the command palette, pickers and dialogs now show at once instead of fading in.
- The find box, the sidebar and the panel appear instantly when opened with their shortcuts.
- Switching tabs or sidebar views no longer slides or fades the marks and the view.
- The selected row in quick open and pickers moves at once with the arrow keys.
- Vim's mode badge changes color at once on `i` and `Esc`.
- The theme picker shows all its rows at once, and moving through it switches the theme without a fade.
- Settings pages now fade in quickly without rising, and pressed buttons sink a little less.
- Mermaid diagrams copy as an image from their hover buttons or the opened diagram's header.
- Mermaid diagrams export as PNG or SVG from their export button, in the colors of the current theme.
- `.mmd` previews copy and export their diagram with **Copy Mermaid Diagram as Image** and the export commands.
- Pinching a touchpad zooms images, SVG and Mermaid previews, and opened diagrams around the pointer.
- On Windows, Ctrl with two-finger scrolling now zooms a picture instead of doing nothing.
- **View → Theme → Claude** now looks like Claude Code in a terminal, with the whole window in the code's font.
- Under Claude, the status bar shows Claude Code's ✻ spinner, the work's name and its seconds while Git runs.
- Claude's errors and removed lines are rose now, apart from its orange, and chosen rows are periwinkle.
- In Claude Light, the terminal's white and bright white text is readable on the light background.
- **View → Compact Folders** shows chains of single-child folders on one Explorer row.
- The Explorer header's context menu, the settings window and `explorerCompactFolders` toggle it too.
- Hover a compact row to see its whole path when the sidebar cuts it off.
- `⌘F` in a terminal opens a find bar that searches its output and scrollback, highlighting each match.
- In the terminal's find bar, `Return` goes to the match above and `Shift+Return` to the one below.
- On Linux and Windows, `Ctrl+F` in a terminal opens find instead of reaching the shell.
- Under Purple Gold, the welcome page drops the mosaic apse and arch and matches the other themes.
- In the find bar, `Shift+Return` now goes to the previous match instead of the next one.
- In the find bar's replace field, `Shift+Return` replaces the match and goes back to the one before it.
- In **Search**, `Shift+Return` in the query selects and previews the previous result, wrapping to the last.
- The Qoder theme lays the editors and terminal on a rounded card, in a grained shell without dividing lines.
- In the Qoder theme, dragging a sidebar or panel edge lights the edge only around the pointer.
- The Qoder theme titles the sidebar, Source Control and the terminal in sentence case, as Explorer.
- The Qoder theme colors code in Qoder's own chart colors: green, blue, violet, amber and rose.
- In the Qoder theme, Qoduck holds up a magnifier while searching and a laptop while Git works.
- Qoduck wears a headset while AI writes a commit message, and raises a megaphone when a toast appears.

### 0.16.1

- **View → Theme → Purple Gold** is now Porphyry: Tyrian purple chrome, gold leaf and gold hairlines.
- Under Purple Gold, a gold mosaic apse shimmers behind the welcome page, under an arched card.
- Purple Gold has a character, Corona, a jeweled crown that blinks, glances and hops while Git works.
- Purple Gold's code is in amethyst, gold, rose quartz and jade, and the open tab gets a gilded line.

### 0.16.0

- Turn on `editorAgentContext` to show a gutter **+** in ordinary files that collects code with a comment.
- Leave the comment empty and choose **Add to Agent** to collect the code alone.
- Code collected across files goes to the last terminal together with **Send N Items**.
- The **…** button above the editor lists collected items to go to, edit or remove; they survive restarts.
- **Mayoi → Settings…** (⌘,) opens a settings window with every preference by category, and a search.
- Changes in the settings window apply to every window at once, and each one resets with one click.
- **File → Keyboard Shortcuts…** lists every command's keys; double-click one and press its new keys to change them.
- The shortcuts editor warns when other commands use the keys, and keeps the comments of `keybindings.json`.
- A folder can set preferences such as `vim` or `autoSave` for itself in `.mayoi/settings.json`.
- Pick a folder at the top of the settings window to change its own settings rather than yours.
- `settings.json` and `keybindings.json` still open as files from **File → Open Settings (JSON)** and the like.
- **View → Theme** adds three Halloween themes, always dark: Jack-o'-Lantern, Witch's Brew and Blood Moon.
- Jack-o'-Lantern is pumpkin orange by candlelight, with a carved pumpkin on the welcome page and status bar.
- Witch's Brew is plum dark with slime green, and a bubbling cauldron for its character.
- Blood Moon is crimson and old gold on a red-tinged black, with a bat that flaps its wings as it hops.
- The settings window groups each section's settings in cards, and marks the ones you changed.
- The settings sidebar shows how many settings you have changed in each section.
- **Keyboard Shortcuts** shows each key as its own cap, and tags your own keys and removed ones.
- The dialog for new keys shows them large, and turns amber when another command has them.
- **Settings → Appearance** shows every theme as a small preview window; click one to switch to it.
- Font size, weight, line height and window zoom have sliders that show their value as you drag.
- **Settings → Fonts** shows sample code in your font, and it updates as you change the font settings.
- Settings with two or three short choices, such as `appearance`, are a row of buttons instead of a menu.
- Resetting a setting or a command's keys shows a notice with **Undo** for a few seconds.
- **Keyboard Shortcuts** groups commands by menu, and each menu's name stays at the top as you scroll.
- Filter the shortcuts to **All**, **Changed by Me** or **Conflicts**, each with how many it shows.
- Keys that two commands share show in amber with a warning sign in the list itself.
- In the shortcuts list, arrows choose a command, `Enter` changes its keys and `Delete` removes them.
- `⌘F` in the settings window goes to its search, and `↓` goes on from the search to the list.
- The arrow keys switch settings sections, and `→` or `Enter` moves into the section shown.
- **Settings → Fonts** picks the font from your installed monospaced fonts, each shown in itself.
- The font sample previews the font you point at or arrow to in the list, before you choose it.
- Text settings, such as the font, apply when you press `Enter` or leave the field, not at each key.
- A folder's own setting changed in the settings window no longer, now and then, snaps back.
- Headings in the Markdown preview take the color theme's heading color, as the editor does.
- Command-clicking a terminal path or URL followed by Chinese punctuation no longer takes the text after it.
- Switching back to a tab no longer flashes a blank editor for a frame before its text shows.
- When a file changes on disk, the editor stays where you were reading instead of jumping to the caret.
- The caret stays on its line when a file changes on disk above it, rather than drifting to other text.
- Select All no longer flashes the end of the file before the view settles.
- With word wrap on, switching back to a tab returns to the same lines, not lines nearby.
- Switching back to a tab whose caret is out of view no longer flashes a blank editor.
- With word wrap on, Enter, Tab, undo and redo no longer move the text around the screen.
- Pressing Enter or moving a line at the edge of the editor scrolls one line, not a third of the page.
- Clicking or holding near the right edge of a long line no longer runs the text sideways.
- Find scrolls a match at the top of the editor out from under the find bar.
- Deleting the longest line shrinks how far the editor scrolls sideways.
- A tab you switch back to shows the same lines after edits in another column added lines above them.
- With word wrap on, moving a tab to another column keeps the same lines in view.
- The folder button by the branch in source control opens **Switch Worktree…** to change checkouts.
- **Switch Worktree…** finds a checkout by folder or branch; Enter on a new name makes a worktree.
- Typing an existing branch's name in **Switch Worktree…** points to **Switch Branch…** instead.
- **Switch Branch…** lists local branches first, with each branch's last author and time.

### 0.15.0

- The Homebrew install steps add `brew trust sorrycc/mayoi`, which recent Homebrew needs to load the cask.
- Set `terminalBackend` to `"hc"` to run terminals in herdr-core, where shells outlive Mayoi's windows.
- Closing a terminal ends its shell in herdr-core, and `Ctrl+\` goes to the shell rather than detaching.
- Terminals in herdr-core attach again when Mayoi starts, with their scrollback, in their tabs and the panel.
- In a remote folder, terminals in herdr-core run on the host as one of hc's machines.
- Mayoi asks once before adding a host to hc, and its terminals use ssh until then.
- **View → Install hc…** installs or updates `hc` in `~/.local/bin` with herdr's installer, on macOS and Linux.
- With `terminalBackend` `"hc"` and no `hc` installed, the first new terminal offers to install it.
- Mayoi finds `hc` in `~/.local/bin` even when your shell's PATH lacks it, so it works right after installing.
- New terminals in herdr-core open at once and say they are starting, rather than freezing the window.
- A dot on a terminal's tab and panel chip shows that its shell runs in herdr-core; hover it for its address.
- The dot pulses, and a pill over the terminal says what it waits for, while it starts or its machine is away.
- The dot is cut out of the terminal icon by a thin ring, and its color fades as the terminal's state changes.
- A failed add to hc says why, suggesting a login only when hc asks for a password.
- When hc can't run on a host, Mayoi suggests installing it there and asks again on the next terminal.
- Once a terminal reaches herdr-core, its dot flashes and the pill shows a check and **Running in herdr-core**.
- A terminal fades while it waits for its machine, and comes back to full brightness once reattached.
- Hover a terminal's dot while it starts or waits to see what it waits for.
- The status bar shows a spinner while hc installs or a host is added to hc.
- A terminal stays faded while its shell starts in herdr-core, and brightens once it attaches.
- A terminal in herdr-core whose machine is away says its shell runs on there, rather than that it restarts.
- A waiting terminal's dot and spinner settle after a few seconds, so a long wait doesn't keep redrawing.
- The status bar shows a spinner while hc installs or a host is added to hc.
- A terminal stays faded while its shell starts in herdr-core, and brightens once it attaches.
- Mayoi starts faster with terminals of other machines to attach, and waits for a machine that is away.
- What you type in a terminal while it starts in herdr-core reaches the shell once it attaches.
- **Copy hc Address** in a terminal's menu copies its address in herdr-core, for `hc attach` elsewhere.
- The panel's terminals in herdr-core attach again when Mayoi starts, rather than starting new shells.

### 0.14.3

- Mermaid diagrams in a Markdown preview open over the window from their hover button or a double-click.
- An opened diagram zooms with its `−`/`+` buttons or `Cmd`-scroll, and drags to move; `Esc` closes it.
- Bold and italic in a Markdown preview render when CJK punctuation sits next to Latin text.
- Mayoi no longer crashes drawing a terminal after running for hours.
- Crash reports include what Mayoi printed before it crashed, so aborts from macOS can be tracked down.

### 0.14.2

- The color theme box opens scrolled to the theme you're using.
- `Cmd+A` in the editor selects all without scrolling to the end of the file.
- A branch with a GitHub pull request shows `#123`, its checks and review in Source Control and the status bar.
- Click the pull request badge to see its changes in a tab; right-click it for **Open on GitHub**.
- **Git → Open Pull Request…** lists open pull requests, or opens any one by its number; it needs `gh` signed in.
- A pull request tab shows the same changes as GitHub, inline or side by side, with its state and branches on top.
- Check **Viewed** on a file of a pull request to fold it and mark it viewed on GitHub, as its own checkbox does.
- When new commits reach the pull request, its tab offers **Reload** to show them.
- Commits that head an open pull request show a `#123` chip in the history; click it to open its changes.
- **Git → Show Pull Request Checks…** lists the CI checks of the branch's pull request and how each went.
- Click the badge's checks icon, or right-click the badge for **Show Checks…**, to open the same list.
- **Send to Terminal** on a failed job pastes the end of its log into the last terminal, without pressing Enter.
- **Re-run** runs a failed job again; **Copy Log** and **Open on GitHub** are there too.
- **Git → Open File on Remote** opens the file on GitHub, GitLab or Bitbucket at the caret line or selected lines.
- **Git → Copy Permalink** copies a link to the selected lines at the current commit.
- Cmd-click or Shift-click commits in **Source Control** to select several; Shift+arrow keys extend the range.
- Press `Cmd+C` in the commits list to copy the short hashes of the selected commits, one per line.
- Both are also in the tab and Explorer file context menus.
- **Stash Changes…** and **Stash Including Untracked…** in the Source Control **⋯** menu stash your changes.
- The **⋯** menu's **Stashes** submenu applies, pops or drops one of the last ten stashes; dropping asks first.
- **Git → Stashes…** lists every stash to search, with ⏎ to apply, ⌘⏎ to pop and ⌘⌫ to drop.
- Tall Mermaid diagrams in a preview show their boxes and arrows, not just their labels.

### 0.14.1

- **View → Theme → Purple Gold** is always dark: a near-black sidebar and a royal purple status bar.
- The Purple Gold theme uses classic gold for buttons, chosen rows and highlights, and golds and lavenders for code.
- The Cursor theme's current line, word highlights, find matches and hovered rows now match Cursor's own.
- Commit tooltips in the history no longer pop up over the rows while you scroll.
- Right-click a folder in **Explorer** and choose **Find in Folder…** to search only that folder.
- Bold, italic and strikethrough now render in the Markdown preview when quotes or brackets touch Chinese text.
- Vue files keep their colors after a directive value that prettier wraps over several lines.

### 0.14.0

- **View → Theme → Cursor** takes Cursor's own Cursor Light and Cursor Dark, as the Appearance is.
- The Cursor theme colors the code and the terminal as Cursor does, with its blue for buttons.
- Press `⌘⏎` in quick open to open the chosen file in the column to the side, splitting one if needed.
- The Markdown preview shows `[[note]]` and `[[note|label]]` wiki links, and a click opens the note.
- Wiki links find their note next to the file, at the repository's root, or anywhere in it, as Obsidian does.
- `[[note#heading]]` opens the note at that heading, as do Markdown links to `other.md#heading`.
- `![[image.png]]` shows the image in the Markdown preview, and `![[image.png|300]]` sets its width.
- `⌘`-click a `[[wiki link]]` in a Markdown file's text to open its note.
- The find bar's match count shows in full in every language, even with thousands of matches.
- In Chinese, the find bar's match count reads `1/5`, as in Japanese.

### 0.13.1

- When an update check fails, the update window shows the whole error, with no tooltip over it.
- The panes of a merge wrap long lines when **Word Wrap** is on, and `⌥Z` toggles wrapping for that merge tab.
- Conflict marks, gutter buttons and synced scrolling in a merge follow wrapped rows.
- A conflict taller than its pane keeps its gutter buttons in view while any of it shows.
- A sideways swipe or `Shift`+wheel scrolls the result pane of a merge, as it does the side panes.
- `⌘Z`, `⇧⌘Z`, `⌘X`, `Tab` and `Enter` show their edit at once, without the line highlight flickering.
- Undo and other edits no longer recenter the view when the caret is near its bottom edge.
- With **Word Wrap** on, undo and redo keep the editor where it is scrolled instead of jumping.

### 0.13.0

- **Close Other Projects** asks before closing the window's other projects.
- `⌘F` in a Markdown preview finds text in the rendered page, without switching to the source.
- Matches in a Markdown preview are highlighted, and `⌘G` and `⇧⌘G` scroll to each one.
- Typing and scrolling are smoother in files of Chinese, Japanese or other non-ASCII text.
- Window zoom scales the whole interface, including sidebars and terminals.
- Use `Cmd` on macOS or `Ctrl` on Windows/Linux with `=`, `-` or `0` to zoom or reset.
- Set `zoomLevel` in `settings.json` to save a window zoom level, including decimals.
- The command palette has separate commands for editor font size and image zoom.
- `⌘=` / `⌘-` / `⌘0` now zoom the whole window; editor font size and image zoom moved to the command palette.
- `⌘=` and `⌘-` no longer grow the editor font or zoom an image with focus; bind `editor.fontZoomIn` or `image.zoomIn` in `keybindings.json` for the old behaviour.
- Going to a line, a symbol or a search result briefly lights the line the caret lands on.
- The sidebar and the terminal panel fade in as they open.
- A folder's files fade in as it expands in the Explorer.
- The breadcrumbs' folder box and the account panel ease in, and project tips fade in.
- Typing in long files and scrolling diffs do less work each frame.
- Rows fade in as a group, a folder or a commit opens in **Source Control**, and as files change.
- A file's matches fade in as it unfolds in the search results.
- Scrolling a file of over 6,000 lines colors the lines ahead before they come into view.
- Typing recolors the edited lines with less work, and windows with many tabs draw with less.
- File tabs gain a **Copy Relative Path** option in their context menu.
- **View → Preview Markdown on Open** opens Markdown and `.mdx` in preview mode.
- `⇧⌘V` switches back to the source; new files, search results and line jumps always open as source.
- Restored and reopened Markdown tabs keep their view and scroll position, including offline remote tabs.
- MDX previews omit top-level `import`/`export` blocks, preserving code examples and paragraph text.
- Files dragged from **Explorer**, **Source Control** or **Search**, and folders from **Explorer**, drop into other apps.
- A remote file or folder dragged to another app arrives as its `host:/path` text.
- **File > Install 'mayoi' Command** uses a folder on your shell's `PATH`, or says how to add one.
- Uninstalling removes `mayoi` from every folder it was installed in.
- Install and upgrade on macOS with Homebrew using the `sorrycc/mayoi` tap.
- Homebrew installs the `mayoi` shell command and removes it when uninstalling the app.
- The Homebrew `mayoi` command reports when Mayoi.app cannot be found.

### 0.12.3

- MyGo 0.7.1: Shift-click extends terminal selections, and Powerline's rounded caps fill the line.
- Mermaid nodes and subgraphs with a light `style` fill keep their labels readable in dark themes.

### 0.12.2

- **Source Control** shows a folder's repositories and submodules in one list that scrolls as a whole.
- Repository and change-group headers in **Source Control** stay pinned at the top while their files scroll.
- Each repository header in **Source Control** has its own branch, remote and **⋯** actions.
- A repository's arrow expands or collapses it; clicking its name shows its history.
- Drag the divider between changes and history in **Source Control** to resize them for the folder.
- Submodule rows in **Source Control** say whether the commit reference or the content changed.
- Command-clicking a file path in a local terminal opens it in the editor, even if Git ignores it.
- Terminal paths ending in `:line` or `:line:column` open with the caret at that spot.
- **Open Preview to the Side** opens the preview in the column beside the file instead of adding one.
- The Markdown preview shows footnotes like `[^1]` as raised numbers, with the notes listed at the end.
- Clicking a footnote's number scrolls to its note, and the note's `↩` scrolls back.

### 0.12.1

- A file with a line too long to show, such as a minified `.map`, no longer opens and freezes Mayoi.
- Such a file left open is not reopened at launch, so Mayoi starts again.
- The Markdown preview draws LaTeX math between `$` and `$` as formulas within the text.
- Math between `$$` and `$$`, or in a `math` code block, shows as a centered formula of its own.
- A formula Mayoi cannot draw shows its source in red, and a block of math also says why.
- `previewMath` set to `false` shows math as its source.
- The Markdown preview links bare URLs right after punctuation such as `：`, as GitHub does.

### 0.12.0

- The Markdown preview draws `mermaid` code blocks as diagrams, in the colors of the theme.
- Hover a diagram for **Show Source** and **Copy Code**; one that cannot be drawn says why.
- `.mmd` and `.mermaid` files preview as their diagram with ⇧⌘V, and zoom like an SVG.
- `previewMermaid` set to `false` shows Mermaid blocks as code.

### 0.11.12

- `mayoi run <id>[=<arg>]` runs any command of the window, and `mayoi commands` lists them.
- `mayoi -g file:line` opens a file at a line, `-d` compares two files, and `-n` opens a new window.
- `mayoi --remote host:/path` opens a folder on an SSH host.
- `mayoi -w` waits for its files to close, so it works as `git config core.editor "mayoi -w"`.
- `mayoi` says what went wrong and exits non-zero when it fails.
- Mayoi updates a `mayoi` command installed by an older version as it starts.
- Opened from the Finder or Dock, Mayoi uses your shell's `PATH`, so git hooks find `pnpm` and `node`.
- Fix a crash on **Go → Go to Symbol…** (⇧⌘O) in a file with a line that starts with `func() {`.

### 0.11.11

- The welcome page has a **Star on GitHub** link.
- Fix a crash on clicking a tab of another column while the find box is open.
- Fix a crash on dropping a tab or terminal from one window into another.
- Crash logs from before 0.11.6 are reported as of an unknown version.

### 0.11.10

- The diff toolbar's menus have their own icons, and the split toggle is gone.
- A commit's file panel is hidden until shown with **Git → Show Files of Commits**.
- **⌘F** and **⌥⌘F** select the find box's query, so typing replaces it.

### 0.11.9

- The Grok Bot theme, with Grok Bot on the welcome page and in the status bar.
- **View → Theme → Color Theme…** previews each theme as you move through the list.
- Changing the theme or appearance fades the colors, unless reduced motion is on.
- A commit's tab lists its changed files in a tree beside the diff.
- The arrow by a commit in **COMMITS** lists the files it changed.

### 0.11.8

- **View → New Terminals in Editor Area** opens new terminals as editor tabs.
- The Markdown preview renders HTML as GitHub does, including centered logos and sized images.
- The Markdown preview shows images from the web, such as badges.
- The vim mode badge switches to the new mode at once instead of fading.

### 0.11.7

- Untitled documents are kept on quit and reopen at the next launch.
- Branch switching, unstaging and discarding work with Git older than 2.23.
- Long paths in diff headers shorten to fit narrow columns.
- Files copied in the Finder paste into a terminal as quoted full paths.
- On macOS, holding a key repeats it instead of showing accents.
- Linux: tab stops and crisp text on Ubuntu 22.04 work again.

### 0.11.6

- After a crash, Mayoi offers to send an anonymous crash report.
- Terminal tabs stay clickable when the tab strip overflows.
- The merge view's bars move actions into a **⋯** menu as the window narrows.
- The status bar's branch opens the same branch picker as Source Control.
- A merge's three panes scroll together.
- The merge gutter's bands stay inside the gutter.
- Command-scrolling over a terminal no longer changes its font size.
- Linux and Windows: a title bar menu button replaces the menu bar.
- Light or Dark appearance holds when the desktop uses the other.
- Maximizing the bottom terminal panel fills the window again.
- MyGo 0.6.0: faster redraws, smooth wheel scrolling, Kitty graphics in the terminal.

### 0.11.5

- Discarding a diff block edits the file's document, as one undo step.
- Unstaged diff blocks have an arrow that reverts the block.
- Control-click the arrow to append the other version's lines instead.
- Edit the working tree side of a diff in place with **I** or **Edit**.
- Diff tabs have **Previous File** and **Next File** buttons and a **Side by Side** toggle.
- Change navigation stops at a file's ends before moving to the next file; see `diffCrossFiles`.
- Opening a file from a diff lands on the current change's line.
- Deleted files offer **Restore File**, and untracked files **Remove File…**.
- The **Whitespace** menu filters whitespace changes in diffs.
- **Highlight** and **Context** menus set word or character highlighting and context lines.
- **Synchronized Scrolling** in the Layout menu lets diff sides scroll apart.
- **Line Alignment** in the Layout menu, or `diffAlignedRows`, turns off row alignment.
- Diffs keep their place when folding, wrapping or resizing.
- **Git → Compare Two Files…** diffs any two local files.
- Conflicted files open in a three-way merge view.
- The merge view lists the files in conflict and applies non-conflicting changes automatically.
- Side-by-side diffs have a gutter of bands linking changes, as in JetBrains IDEs.
- Changed words are compared against the paired line, not the same line number.
- With **Line Alignment** off, both sides of a scrolled split diff stay visible.
- Change navigation glides, and change buttons fade with the pointer.
- The merge view shows full lines, scrolls its panes together, and has gutters with accept buttons.
- A Git sync button in the status bar shows incoming and outgoing commits.
- Linux: fix a crash when the menus change (#51).

### 0.11.4

- Mayoi checks for updates hourly and shows them in the status bar instead of opening a window.
- Mayoi signs in to Mon as the Mac's name and no longer piles up sign-ins.

### 0.11.3

- The Markdown preview's text can be selected and copied.
- A redesigned Markdown preview, closer to GitHub's.
- The Markdown preview shows GitHub alerts such as `[!NOTE]`.
- Links to headings, as `[Usage](#usage)`, scroll the preview.
- **Open Preview to the Side** shows a Markdown or SVG preview in the next column.
- An empty Markdown document's preview says there's nothing to preview.
- Diffs highlight changed words correctly below added or removed lines.
- The Markdown preview parses and draws faster.
- Fitted images and SVG previews cast a soft shadow.
- The Markdown preview scrolls with its text in another column, and the other way round.
- Switching between a Markdown file's text and preview keeps the place.
- Markdown tables are as wide as their text, with round corners.
- The Markdown preview's status bar shows the word count and reading time.
- Images the preview can't show appear as cards with their alt text.
- Heading spacing in the Markdown preview matches GitHub's.
- Images zoomed past their size show crisp pixels.

### 0.11.2

- New Terminal in Editor Area skips locked columns.

### 0.11.1

- Source Control lists the Git repositories inside a non-repository folder (#17).
- Windows: submodules are named with `/`.
- Fix dragging a project's square along the rail.

### 0.11.0

- Settings sync no longer undoes a change made while it syncs.
- Windows: many path fixes for quick open, rename, the Markdown preview and Source Control.
- Windows: terminals are named after their shell.
- `~` shortens only the home folder itself, not folders whose names start the same.
- The Windows fixes follow @wowyuarm's port (#40); tests now run on macOS and Windows.
- The Markdown preview's column is 120 characters wide instead of 80.
- Vim mode records and plays macros: `qa`, `@a`, `@@`.
- Vim mode has registers, `"a`–`"z` and the special ones; `:reg` shows them.
- Vim mode has marks and a jump list; `:marks` lists them.
- Vim mode adds `:normal`, `:g`, `:v`, `:m`, `:t`, `:pu`, `&` and `g&`.
- Vim mode adds `it`, `at`, `[(`, `[{`, `])`, `]}`, `gp` and `gP`.
- Vim mode repeats inserts with a count, as `3ix`.
- Vim mode: ⌃W and ⌃U delete in insert mode on macOS.
- Vim mode: `ma` sets a mark.
- Vim mode's search highlighting does less work per frame.
- Vim mode shows a half-block cursor while an operator waits, and flashes yanked text.
- Vim mode has `/` and `?` search with live highlighting and match counts.
- Vim mode's `:` runs commands such as `:w`, `:q`, `:s`, `:sort` and `:vs`.
- Vim mode adds `gv`, `gi`, `gJ`, `H M L`, `zz zt zb`, ⌃A, ⌃X, `ZZ` and `ZQ`.
- Vim mode matches vim in more edge cases, such as `2D`, `d}` and `di(`.
- Vim mode adds `ip`, `ap`, `3V`, `3v`, and `.` after visual changes.
- Vim mode: `p` over a selection yanks what it replaces.
- Esc after `o`, `O`, `S` or `cc` with nothing typed removes the indent.
- Vim mode's keys work on non-Latin keyboard layouts.
- Find, quick open and the command palette shake when Return finds nothing.
- The find box, change peek and review comment box slide up as they close.
- Find and search fields fade their focus border.
- Lists, buttons and the sync panel fade their hover colors and press down.
- Hover-only buttons fade in and out.
- Splitter lines show after the pointer rests on them.
- Empty-state badges pop in, and a commit that leaves nothing ends with a check.
- Switching sidebar views fades in.
- Source Control's group counts roll to their new numbers.
- Tabs and terminal chips animate as they open, close and move.
- Buttons, tabs, breadcrumbs, the status bar and the welcome page fade their hovers.
- The sidebar's view marker and the picker's selection slide.
- Chevrons rotate as sections open and close.
- Change counts and find's match count roll to their new numbers.
- The vim badge, branch name and title bar cross-fade as they change.
- The welcome page animates in.
- Returning to a wrapped file's tab keeps its scroll position.
- The title bar's box no longer leaves a gap after a short name.
- The welcome page's rows are all one height.
- The vim mode badge in the status bar is colored by mode.
- The command palette shows a ⌘ icon.
- The welcome page lists six recent folders, then **More…**.
- Source Control's message box grows with the message, up to eight lines.
- Cut with no selection cuts the whole line.
- Fix splitting a diff to the right closing the window (#20, #39).
- Quick open and the command palette rank consecutive letter matches higher.
- The Markdown preview underlines large headings and mutes quotes, as GitHub does.
- Quick Open has its own icon on the welcome page.
- Click a folder in the breadcrumbs to browse its files (#38).
- Light themes meet WCAG AA contrast for Git status colors and errors.
- A missing `fontFamily` or `terminalFontFamily` falls back to the system monospace font.
- Breadcrumbs of files outside the folder show `~` and no empty segment.
- Typing in long files re-highlights less.
- Source Control lists each submodule's changes.
- The ✨ button writes commit messages with AI when signed in to Mon.
- **Sign In to Sync Settings…** is now **Mayoi → Sign In…**.

### 0.10.0

- Settings sync: sign in to sync `settings.json` and `keybindings.json` between machines.
- Settings sync shows its status in the status bar and an account panel.
- `syncIgnored` keeps chosen settings on one machine.
- Settings sync retries on its own and explains errors.
- Conflicting `keybindings.json` changes open side by side to merge.
- Mayoi speaks Simplified Chinese and Japanese (#33).
- Commit comparisons open in a diff tab instead of a patch.
- Right-click the terminal panel's header to move, maximize or hide it.
- Source Control's **Remotes** shows where each remote points.
- Fix "No remotes" flashing when switching back to **Remotes**.
- Commits keep their time visible in a narrow sidebar.
- The terminal panel can sit at the bottom, left or right; see `panelPosition`.
- The Explorer and Source Control tree indent less.
- Linux: fix a startup crash on GPU machines (#24).
- Linux: text is crisp again with subpixel antialiasing.
- Linux: tabs indent to tab stops.
- Linux: submenus of the menu bar open correctly.
- Windows and Linux: shortcuts show as key caps.
- Linux: `install.sh` and the Debian package add the `mayoi` command.
- Opening a branch from another worktree works with Git before 2.36.
- Canceling a remote paste leaves nothing behind.
- Fix Git work from a previous folder landing in the new one's Source Control.
- Source Control's **Repository** section manages branches and remotes.
- Clicking anywhere on an Explorer row opens the file, and arrow keys preview files.
- Review comments: comment on diff or file lines and send them to the terminal.
- A **+** button in diffs adds a review comment.
- Diffs with review comments offer **Send N Comments**.
- Files show review comments after their lines.
- Redesigned review comment cards with an **Outdated** badge.
- Review comments use the window's font on a raised card.
- Review comments are kept with the session.
- `n` and `N` in a diff go between review comments.
- Deleting review comments can be undone.
- Review comments in files show on a tinted chip.
- Lock a column to keep it for terminals.
- Source Control supports multi-select with Shift and Cmd.
- The Explorer and Source Control tree draw indent guides.
- The Markdown preview draws task checkboxes and softer bullets.
- Fitted images have a margin and hairline border.
- Painting allocates about a quarter as much.
- Windows: background git no longer flashes console windows.
- MyGo 0.5.5: system double-click speed, and image paste in terminal programs.
- A commit's menu saves, copies, compares and opens it on GitHub, GitLab or Bitbucket.
- **File → New Window** (⇧⌘N) opens an empty window.
- macOS: drop files on the Dock icon, and use Mayoi from **Open With** (#28).
- Middle-click closes a tab.
- Fix a crash when files leave a scrolled diff.
- **Project Tabs** shows the rail with a single project too.
- The rail always shows project icons; **Project Icons** is gone.
- Remote folders show a server badge on the rail.
- Rail labels use two letters to tell similar names apart.
- **Rename Tab…** and **Tab Color** customize a project's square.
- Drop an image on a project's square, or use **Choose Icon…**, to set its icon.

### 0.9.1

- ⇧⌘E focuses the Explorer.
- Remote SSH: copy, paste, drag, upload and download files between machines.
- Remote SSH: forward ports, including from `localhost` links in a terminal.
- Remote SSH: Explorer folders open faster.
- Remote SSH: saving a file changed on the host asks before overwriting.
- Remote SSH: terminals survive a dropped connection and restart on reconnect.
- Remote SSH: pasting files or a screenshot into a remote terminal uploads them.
- Fix Source Control not showing right after `git init`.
- With Git 2.55+, submodule diffs show their commits again.
- Remote SSH: reconnects automatically after a drop.
- Remote SSH: Connect to Host groups recent hosts and shows progress.
- Remote SSH: the folder picker lists folders by name with `..` and `~`.
- Remote SSH: a disconnected folder's Explorer offers to reconnect.
- Remote SSH: fewer round trips and slower polling in the background.
- **View → Project Icons** shows each project's favicon or logo on the rail.
- Project Icons finds mygo, site, docs and macOS app icons.

### 0.9.0

- Remote SSH: **File → Remote SSH → Connect to Host…** opens a folder on another machine.
- Right-to-left selections extend from where they started.

### 0.8.2

- Selecting text in a terminal copies it; see `terminalCopyOnSelect`.
- Each project keeps its own sidebar and panel layout.
- ⌘V pastes a screenshot into Claude Code in the terminal.
- Fix new files in dot-folders, such as `.changeset`, in Changes.

### 0.8.1

- A redrawn project tab rail, with drag to reorder and folder drop to open.
- A terminal bell or notification marks its project's square.
- An empty folder's Explorer offers **New File…** and **New Folder…**.

### 0.8.0

- The title bar and status bar adapt to narrow windows.
- Inactive tabs no longer lay out every frame.
- **View → Project Tabs** keeps every folder in one window, with a rail.
- The commits menu can merge, rebase, reset, revert and cherry-pick.
- Author initials are centered in their circle.
- Crashes are logged to `crash.log` and the `crashes` folder.

### 0.7.0

- MyGo 0.4.8: shortcuts on non-Latin layouts and faster long files.
- The Qoder theme, with Qoduck on the welcome page and in the status bar.
- Switching to a branch in another worktree offers to open that worktree.
- Better Windows and Linux shortcuts, with VS Code's keys; see `vimCtrlKeys`.
- Custom shortcuts in `keybindings.json`, in VS Code's format.
- New commands to sort lines, camel case, copy or reveal the active file, and set indentation.
- The sidebar's title is no longer cut short.
- Quick open's footer shows its prefixes as keys.
- Destructive buttons such as **Discard** are a deeper red.
- **⇧⌘V** previews Markdown files.
- Sideways trackpad swipes scroll long lines smoothly.

### 0.6.0

- **⌘C** and **⌘V** copy and paste files in the Explorer.
- Double-clicking a file in the Explorer keeps its tab.
- Commits clicked in **COMMITS** open in the preview tab.
- Drag files from the Explorer, Source Control or Search to open them where dropped.
- A commit's tab wraps its message.
- Click the Explorer's empty space to deselect, or right-click it for a workspace menu.

### 0.5.1

- **COMMITS** draws the history as a graph with branches and tags.
- **Show All Branches** shows every branch and tag; see `scmAllBranches`.
- A commit's menu checks it out, creates a branch or tag, and copies names.
- Fix the source control **⋯** menu ignoring clicks past the sidebar.
- A closed folder reopens with its tabs, columns and Explorer state.
- Tabs restore their caret, selection, scroll and word wrap.
- Window state moves from `settings.json` to `state.json`.
- Fix a crash when closing a column's last tab.

### 0.5.0

- The Claude theme, with Clawd on the welcome page and in the status bar.
- **View → Theme** picks the theme and, separately, System, Light or Dark.
- **Search** (⇧⌘F) finds and replaces across files.
- Search results are keyboard-navigable and searches are faster.
- Preview tabs: a single-clicked file opens in an italic tab the next one replaces.
- **File → Open Settings** (⌘,) applies changes on save.
- New font settings, such as `fontWeight`, `fontFeatures`, `lineHeight` and `terminalFontFamily`.
- Zoom resizes terminals in every window.
- **Reveal in Explorer** and **Reveal in Finder** in a tab's menu.
- The welcome page's section titles match the sidebar's.
- Diffs of binary or unchanged files say so in the window's font.

### 0.4.0

- **Go → Command Palette…** (⇧⌘P) runs any command by name.
- Better language detection for `go.mod`, `.sql`, Brewfiles and `.npmrc`.
- The Matrix's welcome page sits on a panel so it reads.
- Line numbers and secondary text are easier to read.

### 0.3.3

- **View → Theme** adds The Matrix.
- Clicking a file or commit in source control focuses its changes.

### 0.3.2

- Drop files from the Finder to open them, or on a terminal to type their paths.
- Primary buttons stay purple on hover and press.
- Quick open, pickers, find and change peek fade in.
- A window without a folder offers to open one in the Explorer.

### 0.3.1

- MyGo 0.4.1: fix select lists jumping and input method highlighting.
- Release files are named `Mayoi.<version>.<arch>.dmg` and `Mayoi.Setup.<version>.<arch>.exe`.

### 0.3.0

- **View → Vim Mode** edits with vim's keys.
- **View → Sidebar on Right** moves the sidebar.

### 0.2.1

- Source control lists the branch's recent commits.
- The commit button says what it will do, and adds **Undo Last Commit**.
- **Switch Branch…** finds, switches and creates branches.
- **View as Tree** lists changes under their folders.
- A clean work tree says so and offers to push or pull.
- The commit summary shows its length past 50 characters.
- A progress bar and toasts show Git commands' progress.
- Stage, unstage or discard one change at a time.
- A file's header sticks to the top of the diff.
- Diff headers show icon, state, rename source and line counts.
- Source control stays fast with thousands of changes.
- **File → Open Recent** lists recent folders, and ⌃R finds one.

### 0.2.0

- Source control: stage, unstage, discard and commit.
- Diffs open in a tab, inline or side by side.
- Diffs fold unchanged lines and wrap long ones.
- One **Changes** tab shows every changed file.
- Pager keys move around a diff.
- Click a gutter mark to see, revert or stage a change.
- The Explorer shows Git status colors.
- The status bar's branch switches, fetches, pulls and pushes.
- The caret's line shows who changed it last.
- ⇧⌘T reopens the last closed tab.
- Highlighting keeps up with typing in large files.
- Large files highlight the visible lines first.
- The tab strip has a bottom edge.
- Readability tweaks to line numbers, find fields and the status bar.

### 0.1.0

- The first release: tabs, columns, file tree, quick open, find, Git marks, terminal and auto save.
- Mayoi updates itself from GitHub Releases.
