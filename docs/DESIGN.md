# Design

How FiveLeaf should feel, and the tests that hold it there. A rule here that no test checks is a rule for review; add
the test when you can. The website has its own, in `website/DESIGN.md`.

The order: it works, then it is easy, then it looks good. Polish never covers a broken interaction.

## Motion

- Every animation takes its length from a token in `cmd/fiveleaf/motion.go`: `motionCaret` (120 ms), `motionCaretJump`
  (220 ms), `motionCaretTrail` (400 ms), `motionTap` (100 ms), `motionLeave` (110), `motionHover` (120),
  `motionShow` (120), `motionMove` (200) and `motionEnter` (240).
  `TestMotionUsesTokens` fails on a length written out anywhere else.
- What a person does a hundred times a day does not move. A dialog opened by a key (the command palette, Quick Open,
  a picker) shows at once (`keyedOpen`); opened with the pointer, it fades in over `motionShow`. The chosen row of a
  list moves with the keys: where it glides, as in Sessions, it starts at once and a key that repeats puts it there.
- Motion says where something came from, or that a state changed. Looking nice is not a reason.
- Coming in and going out ease out; moving across the screen eases in and out. Nothing eases in.
- Going out is shorter and smaller than coming in.
- With Reduce Motion on, nothing moves: colors fade at most (`shake` and `Keyframes` already stop), and the caret
  goes to where it goes at once. In Live Preview,
  blocks and pictures take their new room at once and a task's check fades in without growing.
- Mica's optional background flow keeps the panes and text still. It pauses with Reduce Motion or when
  **Animated Mica Background** is off. Visible windows keep flowing while settings preview the material.
- Something visible changes within a frame of the key or click, even when the work takes longer.
- Smooth caret movement leaves a connected trail of eight following points at 75% of the caret's opacity. Horizontal
  and vertical retention are 0.50 and 0.45 per `motionCaretStep` (1/60 second); fixed steps and a fractional preview keep the path stable
  across display refresh rates and repeated paints. The trail catches up after the caret stops, with a
  `motionCaretTrail` settling limit. Ordinary single clicks glide too, including clicks into rendered Live Preview
  blocks. Keyboard navigation and selection moves glide in source and Live Preview, including Vim and Helix counts,
  word, line and document jumps. Moves beyond three lines or 240 px use `motionCaretJump`; nearby moves use
  `motionCaret`. Selection updates immediately; dragging retargets the visible caret from its current position
  without resetting its trail. Double clicks, scrolling, zooming, loss of focus, or disabling Smooth Caret clears
  the trail immediately.

## Stability

- A state that changes moves nothing else. A tab's title, its neighbours and its close button stay put when the
  pointer comes over it, its file gets unsaved changes, or it becomes the chosen tab (`TestTabTitlesHoldStill`). The
  chosen tab is marked by color, not weight, as a heavier weight widens it.
- Numbers that change in place use tabular digits (`FontFeatures("tnum")`), as the caret's position in the status
  bar and find's count (`TestStatusPositionHoldsItsWidth`).
- Hover never moves what is clicked.
- In Live Preview, moving the caret into formatted prose keeps its delimiters hidden. Headings keep their height
  and spacing (`TestLiveHeadingGaps`), lists keep their indentation, and a task's checkbox stays visible while
  you edit its text. Editing a table cell keeps the grid in place. A block whose source is higher than
  its drawing, as a short formula's, keeps its source's room. The room a block's drawing gives up as the caret goes
  in, and takes as it leaves, glides over `motionMove`, so the text below moves over, with or without a blank line
  after it, and turns back from where it is when the caret does (`TestLiveMarkersHoldStill`, `TestLiveBlockGlides`,
  `TestLiveGlideTurnsBack`, `TestLiveGlideWithoutBlank`, `TestLiveMathHoldsStill`). A callout folded or opened by its
  chevron glides the same way, its chevron turning over `motionHover` (`TestLiveCalloutFolds`). A link's underline
  fills in under the pointer over `motionHover`, and nothing else about it changes (`TestLiveLinkHover`); with ⌘
  held, and over a task's box, the pointer is a hand, and a box's edge darkens. A footnote's label drawn as its
  number keeps the room of its source.
- What loads keeps a place: a picture of the web on its way in Live Preview is a card where it goes, which the
  picture fades in over as its room glides to its own (`TestLivePictureArrives`). A split preview keeps its table,
  diagram or formula visible until its replacement is ready. Actions that depend on the old source positions
  wait for the new drawing. Typing above a table or committing composed text in a cell preserves the contrast
  of unchanged cells (`TestLiveTypingKeepsRenderedTableVisible`).

## States

- Each surface shows each of its states: empty, loading, error, offline or reconnecting, and with no permission.
- The state gallery renders surfaces in their states across every theme, light and dark, and every language, into
  one page: `GALLERY=<dir> go test -run TestGallery ./cmd/fiveleaf`, then open `<dir>/index.html`. Add a state to
  `galleryStates` when you add a surface.

## Speed

- `TestBudgets` (`cmd/fiveleaf/budget_test.go`) holds the cost of a frame to a budget: idle, scrolling, typing, typing
  with find open, Quick Open, the palette, 2,000 sessions scrolled, hovered and filtered, and a terminal printing as
  fast as it can. CI runs it on macOS.
  A change that needs more raises a budget in the same commit and says why.
- The figures on the website and in `docs/ARCHITECTURE.md` come from these tests.

## Failure

- A failure says what happened, keeps what the person needs to understand it, and offers the next step.
- Nothing that failed vanishes: a terminal whose shell exits as it starts stays open with its output and offers
  **Report…** (`TestTerminalExitedEarlyStays`).
- **Help → Report a Problem…** opens a new issue with the version and system filled in (`reportURL`) when a repository is configured, or copies the report otherwise. Offer it
  wherever a failure has no next step of its own.

## Words

`TestCopyRules` checks the catalogs in `locales/`:

- Every language has every message, with the same arguments.
- A count before a noun uses `{n, plural, …}`, never `{n} files`.
- An ellipsis is `…`, one character.
- No "please", "successfully", "click here", "are you sure" or "(s)": say what happens.

And, for review:

- Let the layout explain the screen. Drop a subtitle or helper text that restates a label; if a screen needs one,
  change the layout. Keep labels of fields and the accessible names of icon buttons.
- One verb for one action across the app.
- Confirm only what cannot be undone.
- A time within a week reads as how long ago ("3 days ago"); older, as a date ("Mar 4", "Mar 4, 2024").

## Look

- The page is flat: shadows only for what floats (`floating`), no box in a box.
- An inner radius is the outer one less the padding between them.
- The accent is for the one thing that matters on a screen.
