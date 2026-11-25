# Features

## Explorer

A lazy file tree that follows the disk, with Git-ignored files shown faint. You can create, rename, trash and reveal files
from it. In a Git work tree, a changed file shows its letter (M, A, D, U for untracked, C for a conflict) and its name in a
color, and a folder with changes inside gets a dot. While the pointer is over the sidebar, the folder's row at the top
has New File, New Folder, Refresh and Collapse Folders.

A click on the space below the files, or Escape, unselects the file or folder chosen, so new files go in the workspace's
folder. A right click there opens the workspace's menu: New File…, New Folder…, Reveal in Finder, Open in Terminal and
Paste.

To compare two local files, right-click the first and choose **Select for Compare**. Right-click the second and choose
**Compare with Selected**. The marked file appears on the left and stays marked for the next comparison.

**View → Compact Folders** shows a chain of folders that each hold only one folder on one row, as
`src/main/java/com/example`. The row's actions use its last folder.

## Editor

Tabs, line numbers, a highlighted caret line, and Chroma syntax highlighting. Files changed on disk reload unless you
have unsaved edits. The caret glides to nearby places over 120 ms with a connected trail that stretches and bends
behind it. Mouse clicks, keyboard navigation and selection moves glide too, taking 220 ms for distant places.
This includes Vim and Helix counts, word, line and document jumps, in source and Live Preview. Selection updates
immediately; dragging keeps the caret and trail moving toward the selection's endpoint. The brighter trail catches
up when the caret stops. Double clicks, scrolling, zooming, loss of focus, or reduced motion clears it immediately.
**Settings → Editor → Smooth Caret** turns both off.

**View → Line Numbers**, also in **Settings → Editor**, chooses absolute or relative editor line numbers. Relative
numbers show the distance from the caret's line, which keeps its absolute number. The choice is saved for all windows.

A file clicked in the Explorer, or chosen in search's results, opens in a preview tab, its name in italics: the next
file chosen takes its place, so looking through files leaves one tab. Editing it, double-clicking it or its tab,
pressing Enter on it, or dragging it to another column keeps it open. A file already in a tab shows there. Each column
has its own preview tab, which comes back as one at the next start. A commit clicked in source control opens in the
same preview tab.

**File → Compare Two Files…** opens two local files in a diff tab, including files outside a Git repository.
Select both files in the picker. Each changed block has **Accept Left** and **Accept Right** arrows to copy that
block to the other file. You can undo the edit or save it from the file tab.

⇧⌘T reopens the last tab you closed, in its column and place, with its caret and scroll position. Press it again for
the one before. Closing a terminal ends its session and does not add it to this history. Opening or closing a folder
forgets the tabs closed.

The middle mouse button closes the tab under the pointer, wherever in the tab it is pressed, without showing it first:
clearing tabs leaves the file you are reading where it is.

### Word wrap

Long lines scroll sideways by default. **View → Word Wrap** wraps them at the edge of the editor. ⌥Z toggles wrapping
for the current tab only, until the setting changes again.

### Auto save

A changed file saves:

- one second after the last edit
- when its column switches to another tab, or the keyboard moves to another column
- when the window loses focus

Closing a changed file saves it without asking. Untitled files wait for ⌘S. Turn this off from **File → Auto Save**.

### Editing

- Tab and ⇧Tab indent and outdent, using tabs or spaces to match the file.
- Enter keeps the indent, and indents one more level after an opening bracket.
- Brackets and quotes close themselves, wrap a selection, and are stepped over when typed again.
- Commands comment lines, move, copy and delete lines, and insert a line above or below.
- Cut (⌘X) cuts the selection, or the entire line at the caret when nothing is selected. Undo restores it in one step.
- Home goes to the first non-blank character of the line.
- The status bar shows the file's indentation and line endings, and you can change either from there.

### Vim mode

**View → Vim Mode** edits with vim's keys. Each tab has its own mode, shown in the status bar, and starts in normal
mode, where the caret is a block.

- **Motions:** `h j k l`, `w b e ge` and `W B E gE`, `0 ^ $ g_`, `gg G`, `H M L`, `f F t T` with `;` and `,`, `%`,
  `{ }`, `[( [{ ]) ]}`, Enter and `-`. Arrows, Home and End move too. A count goes before any of them.
- **Operators:** `d c y`, `> <`, `gc` to comment, and `g~ gu gU` for case. They take a motion or a text object (`iw
  aw iW aW`, `i" a"` and the other quotes, `i( a(`, `i[`, `i{`, `i<`, `it at` for tags, `ip ap`), or apply to lines
  when typed twice,
  as `dd`.
- **Commands:** `x X D C Y s S r J gJ ~ p P gp gP o O`, `i a I A gi` to insert, as many times as a count before them,
  as `3ix`, `u` and ⌃R to undo and redo, `.` to repeat
  the last change, ⌃A and ⌃X to add to a number and take from it, ⌃D ⌃U ⌃F ⌃B to scroll by half a view or a view, ⌃E
  ⌃Y by a line, and `zz zt zb` to put the cursor's line in the middle, at the top or at the bottom.
- **Registers:** `"a` to `"z` before a command yank and delete into a register of their own, and paste from it; `"A`
  appends to it. `"0` holds the last yank, `"1` to `"9` the last deletes of lines, `"-` the last small delete, `"_`
  keeps nothing, `".` the text inserted last, `":` the last command and `"/` the last search. `:reg` shows them.
- **Macros:** `qa` records the keys typed into register `a`, with the status bar saying so, until `q`. `@a` plays them,
  `3@a` three times, `@@` the register played last, and `@:` the last command. A macro stops where a motion fails, as
  in vim.
- **Marks and jumps:** `ma` sets a mark, `'a` goes to its line and `` `a `` to it, and the marks move with the text as
  it changes. `''` goes back to where the cursor jumped from. Jumps, as `G`, `%`, a search or a mark, are kept: ⌃O goes
  back through them and ⌃I (Tab) forward again. `:marks` lists the marks.
- **Definition:** `gd` goes to where the name under the cursor is defined, as the document's language server says,
  as F12 does.
- **Visual mode:** `v` selects characters and `V` lines, and `gv` the last selection again. A selection made with the
  mouse is one too. Motions extend it, `o` swaps its ends, and operators, `r`, `J`, `~ u U` and `p` act on it.
- **Hop jumps:** `gs` labels visible word starts and `gS` labels the first non-blank character of each visible line.
  Type a label to jump; in visual mode the jump extends the selection. `:HopWord` and `:HopLine` open the same hints.
  `:HopChar1` and `:HopChar2` first take one or two search characters, then label their visible matches. Esc cancels,
  and Backspace removes the last search or label character. Completed jumps join Vim's jump history. These are
  built-in Hop-style commands; they do not require Neovim or Lua.
- **Search:** `/` and `?` open a line over the bottom of the editor. The cursor goes to the first match as you type,
  every match is highlighted, and the line counts them. Return searches, Esc goes back, and ↑ ↓ go through the
  searches before. `n N` search again, `* #` search for the word under the cursor, and a search is a motion too, as in
  `d/end`. Patterns are vim's, with `\( \) \| \+ \< \>`, or `\v` for the rest; case is ignored unless the pattern has a
  capital, or `\C`. The matches show until Esc in normal mode, or `:noh`.
- **Commands after `:`:** `:w`, `:q`, `:wq`, `:x`, `:wa`, `:qa`, a line number as `:42`, `:s/old/new/g` with `%`, a
  range as `2,10` or that of the selection, `& \1 \n` in the replacement, `:d`, `:y`, `:j`, `:> :<`, `:sort` (with
  `!`, `i`, `n` and `u`), `:m` and `:t` to move and copy lines, `:pu` to put a register, `:normal` to type keys on
  every line of a range, `:g/pattern/command` and `:v` for the lines that match or do not, `&` and `g&` to repeat the
  last `:s`, `:noh`, `:e file`, `:vs` and `:bn :bp`. A mark's line, as `'a`, is an address too. `ZZ` saves and closes,
  and `ZQ` closes. `:g`, `:normal` over lines and `:%s` undo as one step.

Esc, ⌃[ or ⌃C leaves insert mode. A change and the text typed after it undo as one step. Yanking, deleting and pasting
use the system clipboard unless a register is named, and a yank of whole lines pastes as lines. Where Control is not
the key of shortcuts, ⌃W and ⌃U delete the word and the line before the caret in insert mode. In normal mode, ⌃R redoes rather than opening
recent folders. ⌘ shortcuts work as usual in every mode. Punctuation keys are read as a US keyboard types them. On
Windows and Linux, ⌃F, ⌃B, ⌃R and ⌃O stay Find, Toggle Sidebar, Open Recent and Open in normal mode, and ⌃A and ⌃X
Select All and Cut, unless `vimCtrlKeys` is on.

### Helix keys

**View → Helix Keys**, or `"editorMode": "helix"`, edits with the core of Helix's keys instead. Each tab starts in
normal mode with one character selected, shown in the status bar as NORMAL or INSERT.

- **Motions:** `h j k l`, `w b e W B E ge`, `0 ^ $`, `gh gl` for the line's start and end, and `gg G`, with a count
  before any of them. The selection follows the character the motions move over.
- **Jump to a word:** `gw` labels visible words of at least two characters with two letters. Type both letters to
  select the whole word, or Esc to cancel. Labels stay within the current editor, including Live Preview's visible
  text; folded text and source replaced by rendered blocks are excluded.
- **Insert and edit:** `i a I A o O` insert; `x d D s c y Y` delete, change, yank and paste the selection; `r` replaces
  it with one character, `~` changes its case, and `> <` indent or outdent its lines. `u` and `U` undo and redo.
- **Definition:** `gd` goes to where the selected name is defined, as the document's language server says, as F12
  does.
- **More:** `{ }` move by paragraph, `( ) [ ]` add or remove a surrounding pair, `%` goes to the matching bracket, and
  Space opens the Leader menu.

Helix's multiple selections are not part of this mode yet. Esc, ⌃[ or ⌃C leaves insert mode
with the character before the caret selected, as Helix does. **View → Vim Mode** and **View → Helix Keys** turn each
other off; the older `"vim": true` still chooses vim.

The command palette also lists **HopWord**, **HopLine**, **HopChar1**, **HopChar2** and **Helix: Jump to Word**.
Custom shortcuts can bind `go.hopWord`, `go.hopLine`, `go.hopChar1`, `go.hopChar2` or `go.helixWord` in
`keybindings.json`. Hop commands use Vim mode; the Helix command uses Helix mode. Scrolling, changing the document
or moving focus out of the editor cancels pending labels.

### Leader menu

In Vim normal or visual mode, and Helix normal mode, press Space to show a compact shortcut panel in the bottom-right
corner, above the status bar. The panel lists commands and their next keys while leaving the editor visible.
Choose a group, then press the key beside an action or click it. Esc closes the menu; Backspace or Left returns
from a group. The menu uses FiveLeaf's commands for files, search, code, tabs, windows and Git.

| Keys | Action |
| --- | --- |
| `Space Space` | Quick Open |
| `Space p` | Command palette |
| `Space f` | File commands; `f` opens Quick Open, `s` saves |
| `Space s` | Search commands; `w` searches the workspace, `s` opens symbols |
| `Space c` | Code commands; `d` goes to a definition, `h` shows hover, `f` formats |
| `Space c l` | Language server status and controls |
| `Space b` | Tab commands; `n` and `p` switch tabs, `d` closes the tab |
| `Space w` | IDE editor columns |
| `Space g` | Git changes, branches and worktrees |

Space still types a space in insert mode, search fields and terminals. Pending Vim commands such as `f<Space>`
and `r<Space>` keep their usual meaning. Personal shortcuts take priority over the default Leader key.
**Leader Menu** is also available in the command palette as `go.leaderMenu` for custom keybindings.

`Ctrl-w` opens the window menu directly in these same modes. Its keys operate on IDE editor columns:

| Keys after `Ctrl-w` or `Space w` | Action |
| --- | --- |
| `v` | Split the current view into a new column on the right |
| `h` / `l` | Focus the column to the left / right; stop at the edge |
| `w` / `W` | Cycle to the next / previous column |
| `q` | Close the current column and its tabs; Vim also accepts `c` |
| `o` | Keep only the current column |
| `=` | Balance the widths of the unlocked columns |

You can keep Ctrl held for the next letter, including `Ctrl-w Ctrl-w` to cycle columns. Esc cancels; Backspace or
Left closes the direct `Ctrl-w` menu. Personal keybindings take priority over this prefix, including chords. The
prefix takes priority over an inherited keymap's Close Tab shortcut while the modal editor is ready for a command.

Closing columns uses the existing save, discard and cancel prompts for unsaved documents. The last editor column
stays open as an empty workspace. Locked columns are protected: closing a locked column is disabled, and Keep Only
is disabled when a locked column would prevent it. The window menu does not bind `Ctrl-w s`, `j`, or `k`;
use the tab strip's split picker or the command palette for vertical splits.

### Undo

Each tab keeps its own history. It covers the app's edits as well as typing, so indenting, moving lines, replacing and
reloading from disk can all be undone. Undoing back to the saved text marks the tab clean again.

### Reading aids

- Indent guides, with the caret's block drawn brighter.
- The bracket matching the one at the caret is boxed.
- Other occurrences of the word at the caret are highlighted.
- The selection has rounded corners and shows its spaces and tabs. It stays visible, dimmed, while the keyboard is
  elsewhere.
- Sticky scroll pins the lines that open the blocks in view to the top, and breadcrumbs name the blocks around the
  caret. Click either to jump there.
- A folder in the breadcrumbs, clicked, shows its files in a tree below it, apart from the Explorer's: open its
  folders, and choose a file to show it in that column, with the mouse or the arrows and Enter.

### Git

The gutter marks lines added, changed or removed since the last commit. A strip along the right edge shows these
changes, find matches and the caret position for the whole file.

Click a mark to see the lines as they were at HEAD, in a box under the change. From there you can revert the change,
which is a step of undo, or stage it, which puts only those lines in the index. ⌥F5 and ⇧⌥F5 go to the next and previous
change, and the **Git** menu has the same commands for the change at the caret.

The caret's line shows who changed it last, when, and the commit's summary, a moment after the caret stops. Turn this
off from **Git → Inline Blame**.

**Git → Open File on Remote** opens the file on GitHub, GitLab or Bitbucket at the upstream branch, or at the commit
checked out when the branch has none, marking the caret's line or the lines selected (`#L10-L20`). **Git → Copy
Permalink** copies the same link at the commit checked out. Both are in the tab's and the Explorer's file menus too,
which link the whole file. The remote is the upstream's, else origin, else the first.

## Source control

The button beside **EXPLORER** at the top of the sidebar (⌃⇧G) switches it to the changes of the folder's Git work
tree, with a count of them. Files are grouped as merge changes, staged changes, changes and untracked files.

- **Open Commit Graph** beside the list and tree controls opens the repository’s history in an editor tab.
  Choose **All Branches** to search for a local or remote branch. Selecting a commit shows its details.
  Double-click it or press `Enter` to open its changes. The graph filter applies only to that tab.
- **Fetch** shows how many commits the branch is behind (`↓`) and ahead (`↑`) of its upstream. Clicking either count
  fetches without pulling or pushing. The dropdown beside **Fetch** offers pull, push and, for a branch without an
  upstream, **Publish Branch**.
- **Switch Worktree…** lists the work tree's checkouts, the one open first, by folder and branch. Choosing one
  opens its folder. **Create Worktree…** at the top of the unfiltered list asks for a new branch name; typing a new
  name in the search box offers **Create Worktree** directly. Both make the checkout beside the main one, in
  `<folder>.worktrees`, from the commit HEAD was at when the picker opened. Typing the name of a branch that is
  already there points to **Switch Branch…** instead. A folder that has gone missing is reported, not pruned.
- Click a file, or press Enter, to see its changes with the diff focused, so its keys (`]` `[`, `.` `,`) work at once.
  Shift-click or Shift+Up/Down selects a range of files, ⌘-click (Ctrl-click on Windows and Linux) adds or removes one,
  and ⌘A selects them all. Space, and a selected row's stage, unstage and discard buttons and menu items, act on all the
  files selected; on a row not selected they act on that file alone.
- The buttons on a row open the file, discard its changes, and stage or unstage it. A group's header does the same for
  all its files. Discarding asks first, and moves untracked files to the Trash.
- Write a message in the box at the bottom and press ⌘↵ or the commit button in its lower right corner, which says
  what it will do: **Commit**, **Commit All** when nothing is staged (FiveLeaf asks before staging everything), or
  **Amend Commit**. Changed files save first. Past 50 characters the summary line shows its length, in a warning
  color past 72.
- The arrow beside the commit button has **Commit & Push**, **Amend Last Commit** (which fills in the last message) and
  **Undo Last Commit**, which takes the commit back but keeps its changes staged and its message in the box.
- Choose **Flat List** or **Folder Tree** at the top of the sidebar to change how files appear in each Git group.
- The **⋯** menu by the title has the commit commands, stage, unstage and discard all, fetch, pull and push, Refresh,
  and **View as Tree**, which lists each group's files under their folders. A folder with only one folder in it shows as
  one row, as `internal/git`, and a folder's buttons stage, unstage or discard every file in it.
- **Stash Changes…** in the **⋯** menu puts the changes away, with a message if you give one; **Stash Including
  Untracked…** takes the untracked files too. Open files save first, and the toast's **Pop** button brings the changes
  back. The **Stashes** submenu lists the last ten, each with **Apply**, **Pop** (apply, then delete it) and **Drop…**,
  which asks first. A pop whose changes conflict keeps the stash, and the files show as merge changes.
- **Git → Stashes…** opens a box of every stash, with its branch and age, to search by message or branch: ⏎ applies the
  one chosen, ⌘⏎ pops it and ⌘⌫ drops it. The stash commands are in the **Git** menu and the command palette too.
- A thin bar under the title moves while a Git command runs. A toast says when a commit is made, with an **Undo**
  button, and when a push or a pull is done.
- A clean work tree says so, and offers to push or pull when the branch is ahead of its upstream or behind it.
- A folder outside a repository gets an **Initialize Repository** button.

### Submodules and repository selection

Initialized submodules, including nested submodules, appear as separate,
collapsible sections in one continuously scrolling Source Control list. The
current repository's header stays at the top while its changes scroll beneath
it. The current change group (such as Staged Changes or Changes) stays directly
beneath that header, with its counts and actions, until the next group or
repository pushes it away. Headers combine the repository name, branch or
detached HEAD, and repository actions; a narrow sidebar keeps remote actions in
each repository's **⋯** menu.
Repositories with changes start expanded; clean repositories start collapsed and
can be expanded to show their retained commit draft and a disabled commit button.
The arrow expands or collapses a section; clicking its name selects its history.
Each repository keeps its own commit draft, selection and Git operations.
Staging, unstaging, discarding and committing apply to that section's repository.
Parent entries distinguish a changed submodule commit reference from uncommitted
content inside it. Commit the submodule's files first, then stage and commit the
updated reference in the parent separately. Uninitialized submodules remain
entries in the parent until initialized externally.

With multiple repositories, changes fill the sidebar down to the **Repository** header.
Expand **Repository** to browse branches and remotes below the changes. The two sections then
scroll independently. Changes fit their content up to half the view until you drag the divider.
The workspace remembers its position.

The repository button in the panel header and in the title bar opens a searchable picker of
repository names and paths. **Auto** follows the active file, diff or graph. Choose a repository
to keep it selected as you open other files. **Open Commit Graph** opens that repository's graph
in the editor. Open graphs and diffs stay bound to their repository.
The title bar's repository button sits beside the project name; its worktree and
branch buttons follow the selected repository and are the only worktree and
branch controls in the window.
Destructive confirmation dialogs name their target repository, including in Auto
mode. Saved diff repository paths are relative to the workspace.
Discovery skips repositories without `.gitmodules` and refreshes on Git metadata
or `.gitmodules` changes, rather than enumerating indexes on a timer.
Opening a worktree containing submodules uses the same behavior; other worktrees
are opened separately as before.

### Commits

The editor's **Commit Graph** shows the latest 50 commits across all branches, with colored lanes for merges
and badges for branches and tags. **Show More Commits** loads the next 50. Use the branch filter to follow a
local or remote branch in this tab. Filtering keeps your current checkout.

Select a row to see the commit message, author, date and short hash. Double-click the row, press `Enter`
or choose **Open Changes** to open that commit's diff in its own tab. The tab shows its
message, author, date, hash, branches and tags at the top, then each file it changed from its first parent, as the
**Changes** tab shows them. The button in the diff's toolbar or **Git → Show Files of Commits** shows a panel at the
tab's right, hidden at first, that lists those files in a tree of their folders or by path as **View as Tree** has the
changes: a click, or the arrows, scrolls the diff to a file, and the file at the top of the diff is chosen as it
scrolls. Drag its edge to widen it. The same button or menu item hides it again in every commit's tab.
The commit's menu checks it out (switching to
its branch when it has one, or detaching HEAD at it once asked), creates a branch or a tag there, copies the hash, the
message or the name of a branch or tag, and on HEAD undoes it.

### Diffs

Every file changed shows in one **Changes** tab, as source control lists them: the staged changes first, from HEAD
to the index, then the rest, from the index to the work tree. Each file is under a header with its path and the lines
added and removed. Click a file in source control and the tab scrolls to it; **Open Changes** from the Explorer, a
tab's menu or the **Git** menu does the same for a file. As you scroll, source control chooses the file you are
reading. The diff follows a file as you type in it.

Each header shows the filename first, followed by its directory and Git status. A renamed file also shows its old path.
The counts on the right show lines added or removed, omitting zeros. Click a header to fold its file; a file with more than 2,000
changed lines starts folded. Point at a header, or choose
it, for buttons that open the file, stage or unstage it, and discard its changes. Files are read as they come near the
view, so a long list of changes opens at once.

**Collapse All** in the toolbar folds every file in this diff tab, including a commit's changes. When every file is
folded, the same button reads **Expand All**. Both actions are also in the **Git** menu. On macOS, use ⌥⌘[ to collapse and ⌥⌘] to expand. Both commands can be assigned keys
in **Settings → Keyboard Shortcuts** (`git.collapseAllFiles` and `git.expandAllFiles`); Windows and Linux have no
default keys for them. Folding keeps the current file's header in view. Expanding still reads files on demand.

Only three unchanged lines show around each change. The rest fold into a row that says how many lines it hides, and a
click opens it. Each run of changes starts with an `@@` header naming the lines and the block it is in, as Git writes
it. **More Diff Actions → Show Full Files** shows every line.

Point at a run of changes, or choose a line of it, and its header offers to stage those changes alone, to unstage them
when they are staged, or to discard them. FiveLeaf reads the index and the file again first, so it acts on them as they
are. Discarding edits an open file as a step of undo, and asks first for one that is not open. As you scroll through a
file, its header stays at the top of the view with its buttons, and a click there folds the file.

Removed and added lines show in one column, under one column of line numbers, or side by side with the lines of a
change across from each other. Side by
side, a gutter between the sides joins each change's lines on the left to its lines on the right with a band, which
curves where the sides stand apart, and holds the change's arrows; pointing at an arrow lights the lines it takes. The
**More Diff Actions → Layout** menu chooses side by side, one column, or side by side when each side has room for 80
columns, which is the default. Words that changed within a line are tinted,
and a bar along the left is brightest for the change you are on. Long lines wrap as the editor's do: **Word Wrap**
sets them all, and ⌥Z or **More Diff Actions → Toggle Word Wrap** sets it for just this tab.

Double-click a line to open the file there. The arrows go from change to change; in narrow splits they move into
**More Diff Actions**, leaving the fold button in reach. The status bar says which change
of how many you are on, with the lines added and removed. At the last, or first, change of a file the navigation says
so, and one more press that way goes to the next file with changes, or the last of the previous; **diffCrossFiles** in
the settings has it cross at once, or never leave the file. At the first or last file it says so, and stops. With the
diff focused, keys work as in a pager:

| Key | Does |
| --- | --- |
| `j` `k`, ↓ ↑ | Choose the next or previous row |
| `]` `[` | Go to the next or previous change |
| `.` `,` | Go to the next or previous file |
| Space, `f`, `b`, `d`, `u` | Scroll a page down or up, or half a page |
| `g` `G` | Go to the top or the bottom |
| `z` | Open the folded lines chosen, or fold them again |
| Enter | Open folded lines, fold a file, or open the file at a line |
| `e` | Open the file at the chosen line |
| `s` | Stage the changes of the chosen line or run, or unstage them |
| Shift+`j` `k`, Shift+↓ ↑ | Choose more rows, as do Shift-click and a drag |
| `c` | Comment on the rows chosen, or edit the comment chosen |
| `n` `N` | Go to the next or previous comment |
| Delete | Delete the comment chosen |
| ⌘C | Copy the lines chosen with their path and line numbers |
| Escape | Choose one row again |

### Review comments

To collect code for an agent from ordinary files, set `"editorAgentContext": true` in **Settings**
(off by default). Hover a line and click **+**, or select several lines first, to open the same comment
card used in reviews. While writing the comment, the source is shown with room for the card under those
lines; saving or cancelling returns to editing the source. Use ⌘Enter to save or Escape to cancel.
Leave the comment empty and choose **Add to Agent** to collect just the code. Collect snippets across
files, then use **Send N Items** above the editor to paste them, together with review comments, into the
last-used terminal without pressing Enter. The **…** button lists the collected items for navigation,
editing and removal. With no terminal open, sending copies the batch to the clipboard and keeps it.
Pending items survive restarts and turning the setting off.

Review an agent's changes, then hand it the review in one paste. Choose lines in a diff and press `c`, or click the
**+** by the line under the pointer, or select lines in a file and press ⇧⌘M (**Add Review Comment**), and type a
comment: ⌘Enter keeps it, Escape drops it. In a diff the comment shows under its lines, with buttons to edit or delete
it, and a bar by the line numbers marks its lines. In a file the bar marks them too, and the comment shows after the
last of them: click it, or run the command on its lines, to edit it. The scroll bar marks where each comment is.
Side by side, a comment shows under the side of its lines. While a comment is edited, **Delete** deletes it, and
the toast that says a comment was deleted, or every comment cleared, has **Undo**. Comments work in the diff of a
commit too. The status bar counts them, with a menu of them under their files to go to each. In a diff, `n` goes to
the next comment and `N` to the one before; **Go to Next Review Comment** and **Go to Previous Review Comment** do so
from anywhere, in the order of their files and lines. Going to a comment on lines removed, staged or of a commit shows
it in its diff.

**Send Review Items to Terminal**, from the button in the diff's toolbar, that menu, the **Git** menu or the
command palette, pastes every comment into the terminal used last, without pressing Enter, so you can add to it before
it goes to the agent, and clears them. **Copy Review Items** copies them instead. Each says where its lines are, relative to the folder, then shows
them, as a diff when it was made in one, then the comment, numbered when there are more than one:

````
cmd/fiveleaf/diffview.go:1228-1229
```diff
-	if ev.Kind != ui.InputKeyDown || ev.Mods&^ui.Shift != 0 {
+	if ev.Kind != ui.InputKeyDown {
```
Not here, the check belongs in the section before.
````

A comment follows its lines as the file changes. When they are no longer in the file, it says it is outdated, and
quotes the lines as they were. Comments not sent are kept with the folder's session, so they are there again at the
next start.

Commit menus also offer **Create Patch…** and **Copy Changes (Patch)**, including binary changes and root commits.
**Compare HEAD to Here** and **Compare Here to HEAD** open the changes between the two in a diff tab, inline or side by
side as the changes show, in the chosen direction; **Select for Compare** then **Compare with Selected** compares two
commits from the same repository. A comparison's card names both commits, each a chip that copies its hash, and its tab
comes back with the session. **Open Commit on Remote** supports
GitHub, GitLab and Bitbucket, preferring a unique remote decoration, then the current upstream, origin or the first remote.

### Branches and remotes

Source Control's **REPOSITORY** panel sits below the changes, starts collapsed and remembers whether it is open and its selected tab.
**Branches** lists the current branch first, then the most recently committed branches, with their upstream and ahead/behind
counts. Expand a branch for its history and any incoming or outgoing commits; **Load more** reads another page.
**Remotes** lists the configured remotes, each with where it points. Expand a remote to read its fetch and push URLs and
remote branches.
Panel data loads when it is opened, and branch history loads when that branch is expanded.

Branch rows offer checkout and pull; a remote branch's **Fetch Remote** fetches its whole remote. Context menus offer
branch creation, rename, deletion, merge and rebase, and remote creation, URL editing, removal, fetch and prune.
Deletion refuses unmerged branches. **Refresh** rereads local refs; **Fetch** contacts the remote.

When the branch has a pull request on GitHub, its number, how its checks went and its review show beside the branch in
source control and in the status bar; click it to open its changes in a tab, or right-click it for **Open on GitHub**. It
comes from the `gh` CLI, run where the repository is, a host's for Remote SSH, and shows nothing without `gh` or its
sign-in. It is asked for again every minute while the window shows, every 30 seconds while checks run.

A commit of the history that is the head of a pull request open shows `#123` first among its branches and tags, muted
for a draft; click it to open the pull request's changes in a tab, or right-click it for **Open on GitHub**. The pull
requests come from `gh` the same way, read in the background after the history, again as the refs change and every two
minutes, so the history never waits for them; without `gh` or its sign-in, no commit shows one.

**Git → Open Pull Request…**, also in source control's **⋯** menu, lists the pull requests open, the branch's first;
type to find one by its number, title, author or branch, or type any number to open that one, whatever its state. Its
tab shows what GitHub shows, the changes from where its head left its base to its head, inline or side by side like
any diff, under a card of its state, its branches and **Open on GitHub**. Commits FiveLeaf does not have yet are fetched
from the remote of the pull request's repository to `refs/fiveleaf/pull/<number>/head` and `…/base`, which keeps them for
the tab when FiveLeaf opens again; a pull request from a fork needs no remote of its own. Each file's header has a
**Viewed** box, which marks it viewed on GitHub, as the box on GitHub does, and folds it; the files viewed show
folded as the tab opens, and the card counts them. When the pull request has commits newer than the tab's, the card
offers **Reload**.

**Git → Show Pull Request Checks…**, also **Show Checks…** in the badge's context menu and source control's **⋯**
menu, or a click on the badge's checks icon, lists the checks of the branch's pull request, failed first, and reads
them again every 30 seconds while one runs; type to find one. A failed GitHub Actions job offers **Send to Terminal**
(⏎), which reads the last 3000 lines of its log, without colors or time stamps, and pastes them into the terminal used
last without pressing Enter, as **Send Review Items to Terminal** does, copying them with no terminal open; **Copy
Log**; **Re-run** (⌘⏎), which runs that job again; and **Open on GitHub**, the only one a check outside GitHub Actions
offers.

Source control's top row shows Fetch on the right; the Fetch menu has pull, push, and publish for a branch without
an upstream. Switching waits until open files are saved.

**Switch Branch…**, from the branch in the title bar or the **Git** menu,
opens a box of the branches: the one checked out first, then the local ones, then those of the remotes, each the one
committed to last first. A remote branch stays listed beside a local branch of the same name. Each shows the author of
its last commit, how long ago it was made and its subject. Type to find one by
name, or type a new name to create a branch from the one checked out. **Create Branch…** at the top of the unfiltered
list opens a name prompt. With matching branches, Enter switches to the first match; with no matches, it creates the
typed name. Choosing a remote's branch checks out a local branch that tracks it. Fetch, pull and push are also
available in source control and the **Git** menu.

Pull only fast-forwards. Push sets the upstream the first time. Git never prompts for a password inside FiveLeaf: it uses
your SSH agent and credential helper, and if those fail, the error says to run the command once in the terminal.

## Image viewer

PNG, JPEG, GIF, WebP and BMP files open in a viewer, fitted to the view on a checkerboard.

- Scroll with `⌘` (macOS) or `Ctrl` (Windows/Linux) held to zoom the image, or use **Zoom In Image / Zoom Out Image** in the command palette.
- Click the status bar's zoom percentage or run **Fit Image to View** to fit the image again.
- `⌘=`, `⌘-` and `⌘0` (macOS), or `Ctrl+=`, `Ctrl+-` and `Ctrl+0` (Windows/Linux), zoom or reset the whole window, including the sidebar.
- Double-click toggles between fitted and actual size.
- Drag or scroll to pan an image larger than the view.
- The image reloads when the file changes.

An SVG opens as text. ⇧⌘V, or the button at the end of the breadcrumbs, shows it as an image that updates as you type.

Markdown (`.md`, `.markdown` and `.mdx`) opens in editable Live Preview by default. Click **Source** beside the file path to see the raw Markdown. The same button changes to **Preview** to take you back. `⌥⌘V` also switches the current tab. Choose **View → Open Markdown As** to set the default for newly opened files. Both views share the text, selection and undo history.

Headings, emphasis, links and task boxes stay formatted as you type. Drag or `Shift`-click to select rendered text. Press `⌘1` through `⌘6` to change the heading level, or `⌘0` to turn a heading into a paragraph. On Windows and Linux use `Ctrl` in place of `⌘`.

In **Insert Table**, move right to choose columns and down to choose rows. Click or drag and release to insert the highlighted rectangle. Arrow keys and `Enter` also work. The grid offers 10 columns and 8 rows, and the numeric fields accept up to 100 in either direction. The row count includes the header.

Click a cell to edit it in place. Open **Table** beside the file path while editing, or right-click a cell, to insert a row above or below, insert a column on either side, or delete the current row or column. Removing the header promotes the first body row. The menu offers **Delete Table** when only one row or one column remains. `Tab` moves across columns and `Enter` moves down a row. Hold `Shift` with either key to move back. Save, Undo and Redo work in the grid, including undoing a row or column change in one step.

Choose a language from a code block’s header to highlight its contents, including multiline strings and comments. **Other language…** accepts names supported by the syntax highlighter. Selecting **Plain text** removes the highlighting.

Mermaid flowcharts use amber for decisions and blue-gray for connectors and their labels. In ordinary process flows, light themes use blue, teal and sage for entry paths and sand for shared steps. Dark themes use blue, teal and rose for entry paths, with matching text and a graphite background for shared steps. These flows use layered layout with right-angle connectors and open arrowheads. Image and SVG exports use the current theme’s colors. Click a rendered diagram or display formula to edit its definition, then move the caret out to see the result.

Right-click the text for Undo, Redo, copy and paste, and Markdown formatting. Paragraph headings, bold, italic, strikethrough and inline code format the selection; **Paragraph** changes its block. Links, images, code blocks, tables, lists, tasks and quotes insert or toggle in place. Select text within one line for inline formatting, or use an empty caret to start writing between the marks. Links and images select their destination so it can be typed immediately. These actions are also in **Edit → Markdown** and the command palette, and can be assigned shortcuts in **Settings → Keyboard Shortcuts**. Vim and Helix retain their normal editing modes.

While typing in Markdown, use `⌘B` for bold, `⌘I` for italic, `⌘E` for inline code and `⌘K` for a link. Hold `Shift` with `⌘X` for strikethrough, `⌘K` for a code block or `⌘L` for an image. On Windows and Linux use `Ctrl` in place of `⌘`. Custom keybindings take precedence.

Press `Enter` in a numbered list to insert an item and advance the consecutive numbers below it. A deliberate restart keeps its number. At the start of an item's text, `Backspace` removes the list marker and task box, or the innermost quote marker. Each edit takes one Undo.

Split with `⌘\` to keep two views of the same document side by side. Each pane keeps its own view mode and reading position while the text changes in the other. The status bar counts the words and the minutes to read them in the source.

Live Preview automatically requests remote images over HTTP or HTTPS. It shows a card while an image loads and uses the image's alt text if it cannot be displayed. A path starting with `/` is taken from the root of the file's Git repository, or else of the workspace, as GitHub takes it. HTML comments and front matter are hidden, and other HTML is displayed as text.

## Columns

Click **Split Right** at the end of a tab strip to split that pane to the right. Hover over the same button to choose
left, right, up, or down. Each pane has its own tabs and can be split again. Drag the divider between panes to resize
them; their layout is restored with the session. A file can show in several panes at once. They share its text and undo history, and each keeps
its own caret and scroll position. Drag a tab along its strip, into another column, or to the edge of a column to give it
a new column.

Choose **Lock Column** in a tab's menu to keep a column for terminals, or **Move to Locked Right Column** in a
terminal tab's or panel terminal's menu to move the running terminal there in one step. The lock in the tab strip unlocks
the column. Files, images and diffs opened from the Explorer, Search, Quick Open or Source Control, and terminals from
**New Terminal in Editor Area**, go to the unlocked column that has the keyboard, or the one used last when a locked
column has it. Dragging a tab or file into a locked column still puts it there. Closing files keeps at least one unlocked column beside the locked ones, without widening
them, and ⌘W closes an extra empty column. A locked column stays when its last terminal exits and offers **New
Terminal**. Locks, widths and the last file column come back with each project's session. The command palette has
**Toggle Column Lock** and **Move Terminal to Locked Right Column**, which take keyboard shortcuts.

Files dragged from the Finder open where you drop them: in a column after the tab shown, on its tab strip where the
marker shows, or on the edge of a column in a new column there. Dropped elsewhere, as on the explorer, they open in the
file workspace, avoiding locked columns. A dropped folder opens as `ff <dir>` opens it.
On macOS, files and folders dropped on FiveLeaf's Dock icon open the same way as `ff <path>`.

Files dragged from the explorer, from source control's changes or from the search results open in the same places. A
search match opens with the match selected.

## Terminal

**View → Tasks** runs saved commands, including built-in `lazygit` and `yazi`, in reusable terminals.
Open the task picker with `⌥⌘T` on macOS or `Ctrl+Shift+B` elsewhere; see [Tasks](TASKS.md) for configuration.

Ghostty's emulator (libghostty-vt), in the panel or in a column tab. The panel is below the editors; **View → Panel
Position**, the command palette's **Move Panel to Left**, **Right** and **Bottom**, and `keybindings.json`'s
`workbench.action.positionPanelLeft` and its neighbours, and **Panel Position** in the menu of the panel's header, put
it at a side of the window. There it sits past the sidebar,
so with the sidebar on the same side the panel is the outer one, and dragging its inner edge sizes it. The terminal
button sits at the bottom of the window; its context menu chooses its side independently of the sidebar.
All terminals in a workspace window share one location. **Maximize Panel** moves them into editor tabs and keeps the
selected terminal focused. Click **Restore Panel** at the lower right of a terminal tab, or drag the tab onto the
status bar's terminal button, to dock them at the previous position and size. Other windows are unchanged.

Use **New Terminal** (`+`) to open another terminal. **Split Right** opens an independent shell beside the
selected one. Hover over the split button to choose left, right, up or down, and drag the divider to resize
the terminals. A docked terminal splits inside its panel. A terminal in an editor tab splits its workspace pane.

**Close Terminal** (×) ends only the selected session. The last terminal closes the panel. To hide the panel without
ending a session, use **Toggle Terminal**. New terminals join the others in the window; **View → New Terminals in
Editor Area** (`terminalLocation` in `settings.json`) chooses where the first opens. Terminal colors follow the theme.
Terminals run the local shell or connect over SSH in
a remote folder. Quitting FiveLeaf closes the terminal; restoring a terminal tab starts a new shell.
Files dropped on a terminal type their paths, quoted for the shell. ⌘V with only an image on the clipboard sends an empty
bracketed paste, as VS Code's terminal does, so that Claude Code pastes the screenshot. Selecting text copies it, as
in Ghostty and iTerm2: a drag, a double click for a word, a triple click for a line, Shift with a drag in a program
that takes the mouse, or Select All. `terminalCopyOnSelect` turns it off.

Command-click a file path in a local terminal (Control-click on Linux and Windows) to open it in FiveLeaf, including
files Git ignores. Recognized paths use forward slashes: `/absolute/path`, `~/path`, `./path`, `../path`, or a
relative path containing a slash. A filename with a `:line` or `:line:column` suffix works too; the suffix places
the caret there. Relative paths use the directory the shell reports, or the terminal's starting directory when
it reports none. Windows drive-letter and backslash paths are not recognized yet. Holding Command (Control
elsewhere) over a recognized path underlines it and shows a hand cursor.

⌘F (Ctrl+F on Linux and Windows) in a terminal opens a find bar over it that searches the screen and the scrollback,
ignoring the case of ASCII letters, and highlights the matches as output comes. Return and the up arrow go to the
match above, in older output, and Shift+Return and the down arrow to the one below; ⌘G and ⇧⌘G do the same. Escape
closes it and gives the terminal back the keyboard.

On Windows and Linux the shell has the keys with Ctrl, which edit its line, except the workbench's: Quick Open, the
Command Palette, Find, the terminal and the sidebar toggles, Next and Previous Tab, Alt+1 to Alt+9 and zoom, and every
Ctrl+Shift shortcut but Ctrl+Shift+C and Ctrl+Shift+V, the terminal's copy and paste, and those of the function keys.

## Sessions

**Sessions** in the sidebar, also opened by `view.sessions`, lists the sessions of the coding agents on this machine, the one used
last first: Claude Code's of `~/.claude`, of each `~/.claude-*` folder, of `CLAUDE_CONFIG_DIR` and of the folders
`agentSessionsClaudeDirs` lists, and Qoder's, Codex's and Grok's, under the heads Today, Yesterday, Previous 7 Days and
Older, each with how many it has, which stay at the top as their rows scroll under them. Each row has its title, or what
was first asked, and under it its agent, the Claude Code folder other than `~/.claude` it is of, its project when
listing every project's, and at the end when it was used last. While the view reads the sessions the first time, it
shows the shapes of the rows to come. **This Project** shows those of the window's folder and the folders in it, **All Projects** every
folder's, and the search field narrows them by title, first prompt, folder, agent or id, its words marked in the rows. ↑ and ↓ in the field move the choice,
which stays on its session as the sessions are read again; Tab goes to the list and Esc back to the field. Sessions
that come with a later reading fade in.

**Resume**, Enter or a double click opens the conversation in an editor chat in its original project.
You can read the history and send a follow-up with the same agent. Reopening the chat preserves its draft.
**Continue with…** lets you choose a different agent. **Copy Resume Command** or ⌘C in the list copies
the agent's terminal command. A missing project folder is reported when you open its session.

Agents running in FiveLeaf terminals can report their status (OSC 7501): working, waiting for you, done or failed.
The status appears beside the session that terminal resumed, or the newest matching session in its folder written since
the agent started.

## Quick open

⌘P (Ctrl+P on Windows and Linux) opens a fuzzy file search with recent files first. In a Git work tree it lists
the files Git tracks, so build output stays out.

- Type `:42` to go to line 42.
- Type `@` to list the file's functions, types and headings.
- Type `>`, or press ⇧⌘P, to run any command by name. Each shows its menu, its shortcut and a check when it is on,
  and the commands run last come first.

## Find and replace

⌘F finds and ⌥⌘F replaces, with options for match case, whole word and regular expressions (`$1` refers to a group). In
large files, matches are found in the background as you type.

## Search

The button beside **EXPLORER** with a magnifying glass, ⇧⌘F or **Edit → Find in Files** searches every file of the
folder, as quick open lists them, with options for match case, whole word and regular expressions. The selection, when
it is on one line, becomes the query. Results come in as they are found, a line for each match under a header for its
file, with its line number; click a header to fold it, or **Collapse All** in the title. Click a match, or move to it, and
the editor shows it, selected, in the preview tab, while the keyboard stays in the results; Enter or a double click
keeps it open in a tab and moves the keyboard to it. ↓ goes from the query to the results, where ↓ and ↑ move from
match to match; ↑ on the first match, or Escape, goes back to the query. A thin bar under the title moves while a search runs.

- Choose **Flat List** or **Folder Tree** at the top of the sidebar. Click a folder to fold its results; switching layouts keeps the query and does not change Git's layout.
- Files with unsaved edits are searched as they are in the editor, and the results follow a file as you type in it or
  it changes on disk.
- Binary files and files over 4 MB are passed by, and the search stops at 20,000 matches.
- **⋯** shows **files to include** and **files to exclude**: patterns separated by commas, such as `*.go, web/**`.
  `*` and `?` match within a name, `**` any folders, and `{ts,tsx}` either. A pattern without a slash, as `*.ts` or
  `testdata`, matches a name anywhere; one with a slash matches from the folder's top. A folder's pattern takes the
  files in it.

### Replace

The arrow beside the query (⇧⌘H, **Edit → Replace in Files**) shows the replacement. Each match then shows struck out,
with what replaces it beside it. With regular expressions, `$1` refers to a group.

- **Replace All**, beside the replacement, asks first, then replaces every match.
- The button on a file's header replaces its matches, and the one on a match replaces it alone.
- A file open in the editor changes as one step of its undo. A file not open is written at once, keeping its line
  endings and permissions, and that cannot be undone, which Replace All says before it starts.
- Matches are found again in the text as it is when you replace. A single match that moved since the search is left
  alone, with a message to search again.

## Windows

Each folder gets its own window.

- The title bar shows the project, worktree and branch. The main worktree is labelled `main`, even when its branch
  has another name; a linked worktree shows its folder name. Folders outside Git show only the project.
- Click the project name to search recent projects by name or path. **Open Local Folder…** and **Open Remote Folder…**
  remain available below the list. A hovered or keyboard-selected row offers **Open in This Window**,
  **Open in New Window**, **Reveal in Finder** (the local file manager on other systems), and **Remove from Recent Projects**.
- **Open in This Window** confirms unsaved changes before replacing the current project. If the target is already
  open in another window, its editing state moves here; an existing project tab in this window is selected directly.
- In the project picker, **Open in New Window** uses a separate window even with **Project Tabs** enabled. An already
  open project moves out of a shared project window with its unsaved edits intact, or comes to the front if it already
  has a window of its own.
- Removing a recent project keeps its files and saved session. Remote SSH projects keep their host and path in the
  list; their file-manager action is disabled because they are not local folders.
- Click the worktree or branch in the title bar for its searchable Git picker. Pull request status stays in the
  bottom status bar.
- **Open Folder…** (⌘O) opens a folder in the current window.
- **New Window** (⇧⌘N) opens a window showing nothing, which the welcome page fills.
- **Open Folder in New Window…** (⌥⌘O), or ⌘-clicking a recent folder (or pressing ⌘↵ on it), opens it in a new window.
- **File → Open Recent** lists the last ten folders, except the window's own and any no longer on disk, and has
  **Clear Recently Opened**. On macOS the Dock icon's menu lists them too, under **New Window**.
- ⌃R (or **Open Recent → More…**) finds a recent folder by name: ↵ opens it in the current window, ⌘↵ in a new window.
  When a terminal has the keyboard, ⌃R goes to the shell instead.
- Other folder-opening commands bring a folder's existing window to the front when it is already open.

### Project tabs

**View → Project Tabs** keeps every folder open in one window instead, so you don't switch windows to change projects.

- A rail on the far left has a square for each project, the one shown marked with a pill along the rail's edge, and a
  dot on any with unsaved changes. Its tip gives the project's name, its folder and the keys that show it. The rail
  rests out of sight, slides out when the pointer reaches the window's left edge, and goes back once the pointer
  leaves it. It shows while the window has one project too, with the button that opens another folder after the
  squares.
- Each square shows the project's favicon or logo, or its folder's initials when it has none, found in its
  well-known icon files, `favicon.ico` included, a mygo app's icon (`mygo.json`'s `icon`, or `resources/icon.png`),
  the icon its sources declare (`site/index.html` and `docs/index.html` among them), or a macOS app's `.icns` or Xcode
  AppIcon, also on a connected host over SSH.
- Initials are two letters, and projects that would share them are told apart by the first letter that differs:
  Downloads and Documents show as "Dw" and "Dc". A folder on another machine has a server badge in the square's
  lower corner, gray while its host is connected, amber while connecting and red while not.
- **Rename Tab…** in a square's context menu shows up to two characters of your own, an emoji too, in place of
  its initials or logo; empty brings them back. **Tab Color** tints the square in one of the terminal's colors.
- Drop a picture on a square, from the Finder or the Explorer, or pick one with **Choose Icon…** in its context
  menu, and the square shows it in place of the project's logo: a PNG, JPEG, WebP, SVG, ICO or ICNS, a remote
  project's too, even while its host is not connected. FiveLeaf keeps a copy in `project-icons` beside
  `settings.json`, so the picture may move or go. **Reset Icon** brings the logo back. An icon and a label of your
  own take each other's place.
- Drag a square along the rail to reorder the projects, and drop a folder from the Finder on the rail to add it.
- Clicking a square shows that project, with its tabs, columns, terminals, Explorer and source control as it left
  them, each editor with its caret and scroll position. Each project keeps its own layout: what its sidebar shows,
  whether it is open and how wide, and the panel's height. A new project starts with the layout of the one shown. ⇧⌘] and ⇧⌘[ (**Go → Next Project** and **Previous
  Project**) step along the rail, and ⌃⌘1…9 (Ctrl+Alt+1…9 on Windows and Linux) show one by its place.
- Opening a folder, with ⌘O, ⌥⌘O, Open Recent, the welcome page or `ff <dir>`, adds it to the rail, or shows
  it if it is there already. The `+` after the squares opens one too.
- A square's context menu reveals the folder in the Finder, copies its path, renames, colors or gives an icon to
  the square, and closes the project or the others.
- **File → Close Project**, the square's context menu, or ⌘W with no tab left closes a project, asking about its
  unsaved changes first. Closing the window closes them all, and they open again at the next start, in the rail's
  order.
- Leaving a project saves its files that save themselves. Its terminals keep running out of sight, and one that rings
  its bell or sends a notification (OSC 9 or 777) marks the project's square in the accent until it shows again.
- Turning Project Tabs on gathers the open windows into the one in front, and turning it off parts the projects into a
  window each again.

## Remote SSH

**File → Remote SSH → Connect to Host…**, the welcome page or the command palette opens a folder on another machine
over SSH, with the system's `ssh` and nothing to install on the other side. It works on macOS and Linux; Windows does
not have it yet.

- The box lists the hosts connected to before under **Recent**, each with the folder opened there last, then the hosts
  of `~/.ssh/config`, with those it includes. Each shows the user and the machine `ssh -G` resolves it to. Type to
  filter them, or type `user@host` or `user@host:port` to connect to one it does not list. While it connects, the box
  says as whom, and after a few seconds for how long; Escape goes back to the hosts.
- **SSH Config**, beside the box's field, **File → Remote SSH → Open SSH Config** or the command palette opens this
  machine's `~/.ssh/config` in a tab, in a remote window too. A missing one is made empty, for its owner alone to read
  as ssh wants, and the box lists the hosts of the one saved the next time it opens. A linked one, as dotfile managers
  make it, opens where it points, so saving it keeps the link.
- Once connected, the box lists the folders of the host, from the folder opened there last, or the home folder. Type a
  path (`~` for the home folder), ↑↓ choose a folder, ↵ or Tab goes into it, `..` goes up, and ↵ on the first row opens
  the folder typed. The folder opens as **Open Folder** opens one: in the window if it shows nothing, else in a window,
  or a project tab, of its own.
- FiveLeaf signs in with keys and the SSH agent, never a password, and honors `~/.ssh/config`: users, ports, keys,
  `ProxyJump` and the rest. A host whose key is not trusted yet needs one `ssh` from a terminal first. One connection
  goes to each host (an ssh ControlMaster), shared by every window and command, and ends with the last window of the
  host.
- The Explorer, quick open, search and replace in files, Source Control, blame, the gutter marks and diffs work as in a
  local folder, running `git`, `rg` (or `grep`) and `find` on the host. Terminals are shells of the host.
- The folders in a folder of the Explorer are read along with it, at one round trip, but those the index skips, as
  `node_modules`, so that they open at once, without waiting on the host; one read more than a few seconds before
  shows at once and is read again.
- Files open at once, read in the background, and save in the background: a save goes whole to a temporary file on
  the host and is copied into the file only once it all arrived, so the file keeps its owner, mode and links, and a
  connection lost on the way leaves it as it was. A save first checks, in the same round trip, that the file is as it
  was read: one changed on the host since asks whether to overwrite it or to revert to it, which Undo takes back.
- Changes on the host show as they would locally: `inotifywait` reports them where the host has it (the
  `inotify-tools` package), and otherwise the folders shown and the files open are looked at every 3 seconds, every 15
  while the window is in the background, and at once as it comes back. Without
  inotify, a change within the same second that leaves a file's size alone can go unseen on hosts whose `stat` has no
  sub-second times.
- The status bar shows `SSH: host`, red while disconnected, with a menu to reconnect, disconnect, open another folder
  of the host or close it, and the title bar names the host. **Disconnect** keeps the window and its tabs: the files
  are read-only, with a banner to reconnect, until it connects again, and their unsaved changes stay.
- A connection that drops, as the network goes or the Mac sleeps, does the same, and connects again by itself: after 2
  seconds, then twice as long each time, up to every 30 seconds, and at once as the window comes back to the front. The
  banner says when it tries next, with **Reconnect Now**. A host that refuses the keys, or whose key is not trusted,
  is not tried again until asked. Reconnecting reads the tree, the open files and the status again. The terminals of
  the host stay, with what they showed, and say the connection went; they start again as it comes back, in the
  folder their shell was last in when it says so, with what they showed above.
- Files copied in the Finder, or in the explorer of another window, paste into a folder of the host with ⌘V or
  **Paste**, and files dropped from the Finder on a folder of the explorer copy into it: they go up as one tar stream
  through the connection, land beside the folder, and take their names, as copies when a name is taken, once they all
  arrived. A remote file copied in the explorer puts its path on the host on the clipboard, and pastes into another
  folder of the host as a copy made there, into a folder of this machine as a download, and into a folder of another
  host by way of this machine. **Upload…** in the explorer's menu copies files chosen in a dialog into a folder of the
  host, and **Download…** a file or folder of the host into a folder of this machine, the Downloads folder first,
  with a button to reveal it once it is there. The status bar shows each transfer and how far it got, and a click on
  it cancels one: what arrived of it is taken away.
- A terminal of the host pastes files of the Finder, of the explorer of a local folder, or dropped on it, as copies
  uploaded to a temporary folder of the host, and types their paths there. An image alone on the clipboard, as a
  screenshot, goes up as a PNG file of its own, so that Claude Code running on the host takes it as it does a
  screenshot pasted on the Mac. A file of the host dragged to its terminal types its path on the host.
- Ports of the host forward to this machine, so that a server running there, as a web app in development, opens in
  a browser here. Clicking a link to `localhost`, `127.0.0.1` or `0.0.0.0` in a terminal of the host forwards its port
  and opens it; **Forward a Port…**, in **File → Remote SSH**, the command palette and the status bar's `SSH` menu,
  forwards one by its number. A port goes to the same port here when it is free, else to another, and the status bar
  shows the ports forwarded, with a menu to open each in the browser, copy its address or stop forwarding it. Each
  connection goes through the host's ssh connection, so a forward works again once a host lost connects again, and
  ends as the last window of the host closes.
- Reveal in Finder is left out of a remote folder, and deleting a file deletes it for good, after asking, as a remote
  folder has no trash.
- Recent folders, the windows and the tabs of a remote folder are kept as those of a local one, and a remote folder of
  the last session connects again as the app starts.

## The `ff` command

On macOS, **File → Install 'ff' Command** installs `ff` into `~/.local/bin` when that directory is on your `PATH`, and
otherwise into `/usr/local/bin`, which asks for your password. On Windows, it installs `ff.cmd` in
`%LOCALAPPDATA%\FiveLeaf\bin`, and adds that folder to your user `PATH`; open a new terminal to use it. On Linux,
`install.sh` puts it in `~/.local/bin`, and the Debian package in `/usr/bin`.

If you installed `fiveleaf` from the File menu on macOS or Windows, that command still works. At startup, FiveLeaf adds
`ff` alongside it if the name is free and the directory is writable. You can also choose **File → Install 'ff' Command**.
With Homebrew or a Linux package, `type -a ff` shows which executable your shell finds first.

- `ff .` opens the current folder, in a new window or the one that already has it, or with project tabs in a new
  tab.
- `ff <file>` opens a file in the window whose folder contains it.
- `ff -g <file:line[:column]>` opens a file with the caret at a line and column.
- `ff -d <file> <file>` compares two files, side by side or inline as the diffs are set to show.
- `ff -n [folder or file]...` opens them in a new window, and with nothing, a new empty window.
- `ff -w <file>...` returns once the files are closed, so that `git config core.editor "ff -w"` edits commit
  messages in FiveLeaf.
- `ff --remote <host:/path>` opens a folder on an SSH host.
- `ff run <command>...` runs commands by the identifiers that `keybindings.json` names them by, in order, as
  `ff run view.terminal git.pull`. Some take an argument after `=` in place of what they would ask in the window:
  `file.openFolder=<folder>`, `file.openFolderWindow=<folder>`, `file.saveAs=<file>`, `go.line=<line[:column]>`,
  `go.quickOpen=<text>`, `view.selectTheme=<theme>` and `remote.forwardPort=<port>`. An unknown command, or an
  argument one doesn't take, runs none of them.
- `ff commands` lists the commands, a line for each, with its identifier, name, shortcut and argument, apart by
  tabs.
- `ff settings` lists every preference of `settings.json`, a line for each, with its key, a tab, and its value as
  JSON.
- `ff settings get <key>...` prints the values of preferences, a line each.
- `ff settings set <key>=<value>...` changes them, as `ff settings set fontSize=16 theme=claude vim=true`: a
  value is JSON when it is JSON, and any other text is the text itself, so quotes are only for a text that is JSON
  too, as `theme=""`.
- `ff settings unset <key>...` gives them their value of a first launch again.
- `ff settings set <key>+=<item>` adds an item to a list, as `ff settings set 'fontFeatures+=calt=0'`, and
  `<key>-=<item>` takes one away, rather than a list being written out whole. The last item taken away leaves no list,
  and an item a list has already is no item more of it.
- `ff settings set projectTabStyle.'<folder>'.label=MG` changes one field of a project's look on the rail, and
  `.<folder>` alone the whole look; a field is `label` or `color`, and leaving it empty takes it away. The folder
  needs no quotes, though it may have dots of its own, and may be told from the current folder or from `~`. An icon is chosen in the
  rail's menu rather than from here, and the icon a look has stays as the command changes it.
- `ff settings --folder ...` acts on the current folder's own `.fiveleaf/settings.json`, of the preferences a folder
  can choose, which it lists, and changes, as `ff settings --folder set autoSave=false`. In a subfolder of a project
  it acts on the nearest file up from it, the one the project reads, and `get` prints `null` of a preference the file
  doesn't choose.
- The `settings` commands change the files themselves, so they work while FiveLeaf isn't running; a FiveLeaf running reads
  the file again and applies the change at once. A preference it has none of, a value of the wrong type, a number
  out of its range or a choice that isn't one of its options says what it takes instead, changes nothing, and exits
  with status 1.
- `ff plugin <command>` installs, updates, removes, writes and packs plugins; see [PLUGINS.md](PLUGINS.md).
- `ff --disable-plugins` turns off every plugin but those FiveLeaf comes with, until FiveLeaf starts again.
- `ff --help` lists the options, and `ff --version` prints the version.

A command acts on the window whose folder contains the directory it runs in, else the one in front; `--window
<folder>` picks the window that has the folder open. The command returns as soon as FiveLeaf has done what it asked, but
with `-w`, and starts FiveLeaf if it isn't running. When something goes wrong, it says so and exits with status 1, or 2
for options it doesn't know.

## Deep links

`fiveleaf://file/<path>:<line>:<column>` opens the file at a line and column, in the window whose folder holds it, else
the one in front. `fiveleaf://open?file=<path>&line=<line>&column=<column>` opens the same file with the path in the query.
A path with spaces is percent-encoded by the browser, as `%20`; a `#`, `?` or `%` in a file's name may be left as it
is. A file that isn't there says so in the window in front.

LocatorJS's **Custom link** opens the first form:

```
fiveleaf://file/${projectPath}${filePath}:${line}:${column}
```

The scheme is registered when FiveLeaf is installed. On Windows and Linux a link starts FiveLeaf, or reaches the one
already running.

## Appearance and persistence

The built-in FiveLeaf theme has a rounded sidebar and matching editor, title, tab and status bar backgrounds.
**View → Theme** selects the theme and appearance. Search **Settings → Plugins** to install each additional theme by name.
One Dark Pro, The Matrix, Purple Gold and the Halloween themes are always dark.
The Matrix is its two worlds: the sidebar, tabs, panels and status bar in the real world's gunmetal steel, and the code
and the terminal in the Matrix's greens, each kind of code in a hue of its own and comments in the steel. The whole
window is in the code's font, with file icons in greens. Primary buttons take the red pill and chosen rows the blue;
buttons that lose something take the Sentinels' orange. Mirrored digital rain falls in two depths behind the welcome
page while the window has the keyboard, and holds still without it or when the system asks for reduced motion. The
White Rabbit, with red-pill eyes, takes the logo's place and the status bar's start, and hops while Git works. Claude is Claude Code as it draws itself in a
terminal, in a light and a dark kind: the whole window in the code's font, as The Matrix is, warm near-black or
off-white, Claude's orange for the accent and the buttons, with dark labels on them, and periwinkle for what is chosen,
with errors and what is removed in Claude Code's rose, apart from the orange. The boxes over the window, as Quick
Open's, are outlined in the orange. While a Git command runs, the status bar has Claude Code's spinner: a ✻ that turns
through its glyphs, the command's name with a light running across it, and the seconds it has taken, and after five
seconds now and then one of Claude Code's own words, as Clauding…, in its place. Clawd, Claude Code's character, takes
the logo's place on the welcome page and sits at the start of the status bar: it blinks now and then, and hops while a
Git command or a search of the files runs, and holds still as the rain does, as the spinner does. Qoder is Qoder's
Forest theme: white or green-tinged greys with forest green, in a light and a dark kind, laid out as Qoder's workbench.
The editors and the terminal below them sit on a card with rounded corners, and the title bar, sidebar and status bar
around it are a lightly grained shell with no lines between them. A sidebar or panel edge lights only around the
pointer as you drag it. The sidebar, Source Control and terminal titles are in sentence case, Explorer rather than
EXPLORER, and code is in Qoder's chart colors: green keywords, blue functions, violet types, amber strings and rose
numbers. Qoduck, Qoder's duck, takes the logo's place and the status bar's start as Clawd does, and blinks, bobs and
hops as it does. In the status bar it holds up a prop for the work going on: a magnifier while the files are
searched, a laptop while Git works, and a megaphone for a few seconds
after a toast. Grok Bot is the Grok Bot
app's Sand theme: white or near-black greys, near-black or near-white buttons and its blue for the accent, in a light
and a dark kind, and its code in the warm colors of x.ai's terminal. Grok Bot's face, a round head with two slanted
eyes, takes the logo's place and the status bar's start as Clawd does, looks up as it hops, and now and then glances
to one side and the other. Cursor is Cursor's own Cursor Light and Cursor Dark: near-white or near-black greys with
Cursor's blue for the accent and the buttons, in a light and a dark kind, and its code in Cursor's own colors. It has
no character, so the welcome page keeps FiveLeaf's logo. Purple Gold is Porphyry, the emperor's purple set in gold mosaic:
Tyrian chrome around a deep purple editor, a Tyrian status bar lettered in gold, gold leaf for the accent, the buttons
and the chosen rows, gold hairlines under the title bar and over the status bar, and a gilded line along the tab shown
with a gem at its middle. Its code is in amethyst, gold, rose quartz and jade. Its character is Corona, a crown in gold
leaf set with amethysts and a ruby, which takes the logo's place and the status bar's start as Clawd does, and whose
jewels flash as it hops. Jack-o'-Lantern is soot-black chrome around a
warm editor, pumpkin orange for the accent and the buttons, and a status bar that glows like the lantern's face, with its
code in candle yellow, ember and vine green and witch-purple keywords. Witch's Brew is plum chrome with a witch-purple
status bar and slime green for the accent and the buttons, with its code in violets, slime greens, teals and lilacs.
Blood Moon is near-black with a red undertone, crimson for the accent and the buttons, a dark-blood status bar, and a
scarlet apart from the crimson for what loses something, with its code in reds, old gold and moonlight blue. Each has a
character drawn from cells in its accent, which takes the logo's place and the status bar's start as Clawd does: a
carved pumpkin whose face glows, a cauldron whose brew bubbles, and a bat that flaps its wings as it hops.
**View → Theme → Color Theme…** (`workbench.action.selectTheme` in `keybindings.json`, as `cmd+k cmd+t`), or **Color
Theme…** on the welcome page, opens a box of the themes, each with a little window in its colors and its character: the
window takes each theme as ↑ and ↓ move to it, and ← and → switch the appearance; Return keeps them, and Escape goes
back to those the box opened on. From one theme or appearance to another, every color of the window fades to the new
one's, at once when the system asks for reduced motion.
The sidebar button sits at the bottom left by default, with the terminal button at the bottom right. Right-click either
button and choose **Bottom Left** or **Bottom Right** to choose that button's side independently. Moving the sidebar
button also moves the sidebar; the terminal button's side is independent of the terminal panel. **View → Sidebar on Right** and
**Settings → Appearance → Layout → Sidebar on Right** change the sidebar's choice; **Terminal Button on Right** in the same
settings section changes the terminal button's. Both choices apply to every window and stay after a restart.
It remembers the sidebar's and the panel's sides, the sidebar and panel
sizes, and every window with its folder, frame, columns and tabs, each tab with its caret, selection, scroll position
and own word wrap, and the folders open in the Explorer. Started without a folder, FiveLeaf reopens the windows that were
open when it quit.
**Settings → Appearance → Window → Background Opacity** adjusts the background from 35% to 100%. Lower values reveal the
desktop through frosted glass; text and controls keep their opacity. It applies to the editor, sidebar, panel and
settings window, including card themes. At 100% the background is solid. Systems without a native background material
keep an opaque background.
A theme of your own is a JSON file in `~/.fiveleaf/themes`, named for its id, whose colors, code, terminal colors, type
sizes and layout FiveLeaf reads: **Plugins → Open Themes Folder** opens the folder and writes an example. A plugin can
contribute one too, and `themes` in `settings.json` writes one beside the rest of the settings. See [THEMES.md](THEMES.md).
A recent folder closed, by closing its window, **File → Close Folder** or opening another folder in its window, opens
again with its tabs, columns and Explorer as they were. Closing the last window quits the app.

## Plugins

Plugins are folders of JavaScript or TypeScript in `~/.fiveleaf/plugins`, which FiveLeaf runs in its own engine, each
apart, with no Node and no build step. They add commands, keyboard shortcuts and menu items, status bar items, views in
the sidebar, previews beside the source, formatters, diagnostics, themes (VS Code's too), languages, snippets and
language servers, and they work on SSH folders with nothing installed on the host. See [PLUGINS.md](PLUGINS.md).

- `ff plugin install <git-url or owner/repo>` installs one, and **Plugins → Manage Plugins…** opens the Plugins page
  of the settings window: install, update, remove, turn each on and off, and change its settings.
- **Plugins → Show Plugin Log** and **Show Running Plugins** tell what plugins logged and what they use.
- **Selection → Format Document** (⇧⌥F) uses a formatter plugin, then a language server if it supports formatting.
  Enable Prettier for frontend files, Ruff for Python or ClangFormat for C and C++ in **Settings → Plugins**.
  Go and strict JSON also have built-in formatters. Enable **Format on Save** to format on explicit saves.
- A language server a plugin declares underlines problems, and answers **Go → Go to Definition** (F12 or ⌘-click),
  **Go → Show Hover** and **Selection → Complete Word** (⌃Space).
- In source editors, suggestions appear beside the caret after two identifier characters or a language server’s trigger character.
  Use ↑ and ↓ to choose, Tab to insert, or Esc to dismiss. Enter starts a new line. ⌃Space opens the searchable list.
  If the server has no matches, the editor offers words with the same prefix from nearby text in this file. Toggle automatic
  suggestions in **Settings → Editor → Code Completion**.
- **Show Problems** lists every problem plugins found; the status bar counts them.
- **Selection → Insert Snippet…** inserts a snippet of a plugin's.
- A `.ts` file in `~/.fiveleaf/scripts` makes each function it exports a command; **Plugins → Run Selection as Script**
  runs the selection once.
- Install **Word Count** to show the words in a Markdown file in the status bar, or **TODOs** to show TODO comments in the sidebar.
- Individual language plugins cover common frontend, backend and configuration files. Install the corresponding
  server, then install and enable its plugin in **Plugins → Manage Plugins…**. See [Language servers](LANGUAGE-SERVERS.md)
  for the supported languages, installation commands and SSH setup.

## Settings

**FiveLeaf → Settings…** (⌘,), or **File → Settings…** on Windows and Linux, opens the settings window: every preference
by category, with search, a mark on those changed from their defaults and a button to reset each. A change applies in
every window at once: the code's and the terminal's fonts, weight, ligatures and line height, the theme and appearance,
auto save, word wrap, the diff layout, vim and more.

**File → Open Settings (JSON)** opens `settings.json` in a tab instead. Saving it applies it the same way. FiveLeaf keeps
the windows, tabs, recent folders and layout itself, in `state.json` beside it. See [SETTINGS.md](SETTINGS.md) for
every key and where the files are.

A folder can set some preferences for itself, such as `vim` or `autoSave`, in `.fiveleaf/settings.json`. Choose the folder
at the top right of the settings window, or use **File → Open Folder Settings (JSON)**. See
[Folder settings](SETTINGS.md#folder-settings).

## Keyboard shortcuts

**File → Keyboard Shortcuts…** lists every command with its keys in the settings window. Search by name or by pressing
keys, click a command and press its new keys, and see which other commands already use them. Each change goes to
`keybindings.json` at once, keeping its comments.

**File → Open Keyboard Shortcuts (JSON)** opens `keybindings.json`, in VS Code's format, so a VS Code or Cursor file can
be pasted in. Saving it applies it in every window at once. It binds keys and chords to FiveLeaf's commands or VS Code's
names for them, with `when` conditions. See [SETTINGS.md](SETTINGS.md#keybindingsjson) for the format, and for the
shortcuts that differ on Windows and Linux.

Some commands are only in the command palette: **Copy Path of Active File**, **Copy Relative Path of Active File**,
**Reveal Active File in Finder** (File Explorer on Windows), **Indent Using Tabs** and **Indent Using 2, 4 or 8 Spaces**. **Selection → Sort Lines
Ascending**, **Sort Lines Descending** and **Transform to Camel Case** work on the lines selected, or every line, and on
the words selected, or the caret's.

## Updates

Release builds with an update feed check GitHub Releases every hour, starting 10 seconds after launch or wake. The
update icon in the upper-left corner highlights a new version. It opens the same panel as the welcome page and
**FiveLeaf → Check for Updates…** (**Help → Check for Updates…** on Linux and Windows).

The panel shows release notes and download progress. FiveLeaf downloads only what changed when it can, verifies
the update signature and runs the new version after a restart. Restart follows the usual process for saving changes.
If a GitHub update fails, you can retry, check again or download the installer from the publisher's release page.
Development builds and system-managed installations use their usual build or installation method to update.

If the publisher sets a minimum version and FiveLeaf finds an update that meets it, the update panel takes the
place of the workspace until you install and restart or the publisher relaxes the requirement. You can still save
open files and quit. The requirement starts with a successful check and stays active for the running session
through temporary connection failures. If you already downloaded an update before the minimum rose, restart to
continue updating. See [Releasing](../CONTRIBUTING.md#require-a-minimum-version) for the publisher settings.
