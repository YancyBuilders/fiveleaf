package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiveleaf/locales"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestAgentAttachmentDropAndImagePreview(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 820)
	chat := hs.a.agent
	chat.draft = "Keep this question"
	path := hs.path("red-icon.PNG")
	if err := os.WriteFile(path, pngBytes(t, 480, 260), 0600); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	input := hs.detailBounds(locales.AgentDraft())
	hs.tt.DropFiles(input.X+input.W/2, input.Y+input.H/2, path)
	hs.settle()
	hs.until("dropped file", func() bool { return chat.contextLoading == 0 })
	if chat.sources.draft.fileIndex(path) < 0 || chat.draft != "Keep this question" || !hs.a.chatShown(chat) {
		t.Fatalf("drop did not attach to the displayed draft: %q, %v", chat.draft, chat.detailSources())
	}
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(filepath.Base(path)))
	draftBounds := hs.detailBounds(locales.AgentDraft())
	redPixels := func() int {
		frame := hs.tt.Image()
		count := 0
		for offset := 0; offset < len(frame.Pix); offset += 4 {
			if frame.Pix[offset] == 255 && frame.Pix[offset+1] == 0 && frame.Pix[offset+2] == 0 {
				count++
			}
		}
		return count
	}
	before := redPixels()
	if before < 40 {
		t.Fatal("the draft did not paint the image thumbnail")
	}
	hs.tt.Move(remove.X-8, remove.Y+remove.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Frame()
	if redPixels() < before+5000 || hs.detailBounds(locales.AgentDraft()) != draftBounds {
		t.Fatal("hover did not paint the larger image in place")
	}
	hs.shot("agent-attachment-image")
	hs.tt.Move(input.X+2, input.Y+2)
	hs.settle()
	if redPixels() != before {
		t.Fatal("the enlarged preview remained after leaving the attachment")
	}
}

func TestAgentAttachmentIconsFromExplorer(t *testing.T) {
	hs := newHarness(t, true)
	chat := hs.a.agent
	icon := hs.path("favicon.ICO")
	writeICO(t, icon, pngBytes(t, 32, 32))
	svg := hs.path("vector.svg")
	if err := os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><circle cx="10" cy="10" r="8" fill="red"/></svg>`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{icon, svg} {
		hs.a.addFileToChat(chat, path)
		hs.until("added icon", func() bool { return chat.contextLoading == 0 })
		index := chat.sources.draft.fileIndex(path)
		if index < 0 || chat.sources.draft.refs[index].preview == nil {
			t.Fatalf("no thumbnail for %s: %s", filepath.Base(path), chat.err)
		}
	}
}

func TestAgentAttachmentChipsHaveUniformCompactBounds(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	chat := hs.a.agent
	short, long := "a.go", "a-long-attachment-name-with-more-text.md"
	for _, name := range []string{short, long} {
		path := hs.path(name)
		chat.sources.draft.refs = append(chat.sources.draft.refs, chatTextAttachment(name, path, "example"))
	}
	hs.settle()
	first, second := hs.detailBounds(short), hs.detailBounds(long)
	if first.W != second.W || first.H != second.H || first.H > 26 || first.Y != second.Y {
		t.Fatalf("attachment bounds differ or exceed the compact row: %+v %+v", first, second)
	}
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(long))
	if !second.Contains(remove.X+remove.W-1, remove.Y+remove.H-1) {
		t.Fatalf("remove button escapes the chip: %+v %+v", second, remove)
	}
}

func TestAgentAttachmentUsesEditedSVG(t *testing.T) {
	const saved = `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"/>`
	const edited = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="80"><circle cx="20" cy="40" r="10"/></svg>`
	hs, editor := editHarness(t, "edited.svg", saved)
	hs.a.settings.AutoSave = false
	editor.edit([2]int{}, textEdit{start: 0, end: len(saved), text: edited})
	chat := hs.a.agent
	hs.a.addChatDetailFile(chat, editor.path)
	hs.until("edited SVG", func() bool { return chat.contextLoading == 0 })
	index := chat.sources.draft.fileIndex(editor.path)
	if index < 0 || !strings.Contains(chat.draftPrompt(), edited) {
		t.Fatal("the SVG attachment omitted the unsaved buffer")
	}
	vector, ok := chat.sources.draft.refs[index].preview.(*ui.SVG)
	if !ok {
		t.Fatal("expected an SVG preview")
	}
	if width, height := vector.Size(); width != 40 || height != 80 {
		t.Fatalf("preview used disk dimensions: %v × %v", width, height)
	}
}

func TestAgentAttachmentHoverUsesTextSnapshot(t *testing.T) {
	const snippet = "unsaved preview text"
	hs, editor := editHarness(t, "snapshot.txt", "saved")
	hs.a.settings.AutoSave = false
	editor.edit([2]int{}, textEdit{start: 0, end: 5, text: snippet})
	chat := hs.a.agent
	hs.a.addFileToChat(chat, editor.path)
	hs.settle()
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(filepath.Base(editor.path)))
	hs.tt.Move(remove.X-8, remove.Y+remove.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Frame()
	if !hs.tt.HasText(snippet) || !strings.Contains(chat.draftPrompt(), snippet) {
		t.Fatal("hover did not show the text that will be sent")
	}
}

func TestAgentAttachmentMoveIntoPreviewAndScroll(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 720)
	chat := hs.a.agent
	chat.draft = "keep this draft"
	path := hs.path("long.go")
	var source strings.Builder
	for index := range 200 {
		fmt.Fprintf(&source, "const Line%03d = %d\n", index, index)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	hs.a.addFileToChat(chat, path)
	hs.until("attached long file", func() bool { return chat.contextLoading == 0 })
	hs.settle()
	hs.clickDetail(locales.AgentDraft())
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(filepath.Base(path)))
	hs.tt.Move(remove.X-8, remove.Y+remove.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Frame()
	footer := hs.detailBounds(shortenHome(path))
	hs.tt.Move(remove.X-8, (remove.Y+footer.Y+footer.H)/2)
	hs.tt.Move(footer.X+footer.W/2, footer.Y-20)
	hs.tt.Advance(2 * hoverLeave)
	hs.tt.Frame()
	if !hs.tt.HasText(shortenHome(path)) {
		t.Fatal("the preview disappeared when the pointer entered it")
	}
	before := chat.scroll.Y
	hs.tt.Scroll(footer.X+footer.W/2, footer.Y-20, 0, 20000)
	hs.settle()
	if !hs.tt.HasText("const Line199 = 199") || chat.scroll.Y != before || !hs.tt.Focused(locales.AgentDraft()) {
		t.Fatal("could not scroll to the last line while keeping the chat and draft in place")
	}
	hs.tt.Scroll(footer.X+footer.W/2, footer.Y-20, 0, -20000)
	hs.settle()
	if !hs.tt.HasText("const Line000 = 0") || chat.draft != "keep this draft" {
		t.Fatal("scrolling back did not show the start of the file")
	}
	hs.key(0, ui.KeyEscape)
	if hs.tt.HasText(shortenHome(path)) {
		t.Fatal("Escape did not dismiss the preview")
	}
}

func TestAgentAttachmentScrollTallImage(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 680)
	picture := image.NewRGBA(image.Rect(0, 0, 200, 1800))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	draw.Draw(picture, picture.Bounds(), image.NewUniform(red), image.Point{}, draw.Src)
	draw.Draw(picture, image.Rect(0, 900, 200, 1800), image.NewUniform(blue), image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	path := hs.path("tall.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	chat := hs.a.agent
	hs.a.addFileToChat(chat, path)
	hs.until("tall screenshot", func() bool { return chat.contextLoading == 0 })
	hs.settle()
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(filepath.Base(path)))
	hs.tt.Move(remove.X-8, remove.Y+remove.H/2)
	hs.tt.Advance(hoverDwell + motionShow)
	hs.tt.Frame()
	footer := hs.detailBounds(shortenHome(path))
	x, y := footer.X+footer.W/2, footer.Y-48
	hs.tt.Move(x, y)
	hs.tt.Advance(2 * hoverLeave)
	hs.tt.Frame()
	if got := hs.tt.Image().RGBAAt(int(x), int(y)); got != red {
		t.Fatalf("expected the top of the image, got %v", got)
	}
	hs.tt.Wheel(x, y, 0, 5000)
	hs.settle()
	if got := hs.tt.Image().RGBAAt(int(x), int(y)); got != blue {
		t.Fatalf("wheel did not reveal the bottom of the image: %v", got)
	}
}

func TestAgentAttachmentPickerPasteAndDrag(t *testing.T) {
	hs := newHarness(t, false)
	agentViewFixture(hs)
	sizeChatWorkspace(hs, 600)
	chat := hs.a.agent
	chat.draft = "selected question"
	hs.settle()
	hs.clickDetail(locales.AgentDraft())
	hs.command("selectAll")
	selection := chat.draftState.selection
	if selection[0] == selection[1] {
		t.Fatal("expected a selected draft")
	}
	hs.h.chosen = []string{hs.path("main.go")}
	hs.clickDetail(locales.AgentAttachFiles())
	hs.until("chosen file", func() bool { return chat.contextLoading == 0 })
	hs.h.clipboard = []string{hs.path("go.mod")}
	hs.tt.SetClipboard("go.mod")
	hs.command("paste")
	hs.until("pasted file", func() bool { return chat.contextLoading == 0 })
	readme, input := hs.detailBounds("README.md"), hs.detailBounds(locales.AgentDraft())
	hs.drag(readme.X+readme.W/2, readme.Y+readme.H/2, input.X+input.W/2, input.Y+input.H/2)
	hs.until("dragged file", func() bool { return chat.contextLoading == 0 })
	requireDetailPaths(t, chat, hs.path("README.md"), hs.path("go.mod"), hs.path("main.go"))
	if chat.draft != "selected question" || chat.draftState.selection != selection || !hs.a.chatShown(chat) {
		t.Fatalf("attaching changed the selected draft: %q %+v", chat.draft, chat.draftState.selection)
	}
	hs.h.clipboard = nil
	hs.tt.SetClipboard("plain text")
	hs.command("paste")
	if chat.draft != "plain text" || len(chat.sources.draft.refs) != 3 {
		t.Fatalf("plain text paste failed: %q", chat.draft)
	}
}

func TestAgentAttachmentClipboardImageLifetime(t *testing.T) {
	hs := newHarness(t, true)
	agentViewFixture(hs)
	hs.settle()
	chat := hs.a.agent
	hs.h.image = pngBytes(t, 160, 80)
	paste := func() string {
		t.Helper()
		hs.clickDetail(locales.AgentDraft())
		hs.tt.SetClipboard("")
		hs.command("paste")
		hs.until("pasted screenshot", func() bool { return chat.contextLoading == 0 })
		if len(chat.sources.draft.refs) != 1 {
			t.Fatalf("missing clipboard image: %s", chat.err)
		}
		ref := chat.sources.draft.refs[0]
		got, err := os.ReadFile(ref.file)
		if err != nil || !bytes.Equal(got, hs.h.image) || ref.preview == nil || ref.label != locales.AgentPastedImage() || chat.draft != "" {
			t.Fatal("pasted screenshot lost its PNG or thumbnail")
		}
		return ref.file
	}
	removed := paste()
	hs.a.clearChat(chat)
	hs.settle()
	hs.clickDetail(locales.AgentChatRemoveAttachment(locales.AgentPastedImage()))
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Fatalf("removed screenshot was retained: %v", err)
	}
	sent := paste()
	chat.stageDetailSources()
	chat.recordDetailSources()
	if !chatLocalFile(sent) {
		t.Fatal("sending removed the screenshot before the Agent could read it")
	}
	hs.a.clearChat(chat)
	if _, err := os.Stat(sent); !os.IsNotExist(err) {
		t.Fatalf("clearing the chat left its sent screenshot on disk: %v", err)
	}
}

func TestAgentAttachmentDocumentPreviews(t *testing.T) {
	root := t.TempDir()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entry, err := archive.Create("assets/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("image")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "bundle.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err := readChatAttachment(root, path)
	if err != nil || ref.previewText != "assets/logo.png" || ref.content != path+"\n" {
		t.Fatalf("ZIP preview = %q, error = %v", ref.previewText, err)
	}
	if _, err := readAgentFile(root, path); err == nil {
		t.Fatal("the text-file protocol accepted binary contents")
	}
	if _, err := readChatAttachment(t.TempDir(), path); err == nil {
		t.Fatal("a binary attachment bypassed the workspace guard")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return
	}
	buffer.Reset()
	buffer.WriteString("%PDF-1.4\n")
	var offsets []int
	for index, obj := range []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Resources << >> >>"} {
		offsets = append(offsets, buffer.Len())
		fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", index+1, obj)
	}
	xref := buffer.Len()
	buffer.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	path = filepath.Join(root, "diagram.pdf")
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err = readChatAttachment(root, path)
	bitmap, ok := ref.preview.(*ui.Bitmap)
	if err != nil || !ok || ref.content != path+"\n" {
		t.Fatalf("PDF first page missing: %v", err)
	}
	if width, height := bitmap.Size(); width != chatPreviewSide || abs(height-width/2) > 1 {
		t.Fatalf("first page lost its aspect ratio: %d × %d", width, height)
	}
}

func agentAttachmentPreviewFixture(hs *harness) {
	agentViewFixture(hs)
	icon := hs.path("FiveLeaf.png")
	if err := os.WriteFile(icon, iconArt, 0600); err != nil {
		hs.t.Fatal(err)
	}
	chat := hs.a.agent
	hs.a.addChatDetailFiles(chat, []string{icon, hs.path("main.go")})
	hs.until("draft previews", func() bool { return chat.contextLoading == 0 })
	hs.settle()
	remove := hs.detailBounds(locales.AgentChatRemoveAttachment(filepath.Base(icon)))
	hs.tt.Move(remove.X-8, remove.Y+remove.H/2)
	hs.tt.Advance(time.Second)
	hs.tt.Frame()
}

func TestShotAgentAttachmentPreview(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip("SHOTS is not set")
	}
	language := i18n.Locale()
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(func() string { return language }) })
	hs := newHarness(t, true)
	hs.tt.SetLocale("zh-Hans")
	sizeChatWorkspace(hs, 720)
	hs.a.agent.draft = "帮我看看这个图标。"
	agentAttachmentPreviewFixture(hs)
	hs.shot("agent-attachment-preview")
}
