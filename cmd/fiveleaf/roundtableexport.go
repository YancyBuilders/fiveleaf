package main

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fiveleaf/internal/bg"
	"fiveleaf/internal/roundtable"
	"fiveleaf/locales"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func (w *roundtableWindow) exportMeeting() {
	meeting := w.meeting
	if meeting == nil || w.creating {
		return
	}
	title, project := meeting.group.Topic, meeting.library.Project
	members := make([]string, 0, len(meeting.group.Members))
	for _, member := range meeting.group.Members {
		members = append(members, exportLabel(member.Name))
	}
	heading := "# " + exportLabel(title) + "\n\n" + locales.RoundtableMembers() + ": " + strings.Join(members, ", ") + "\n"
	history := meeting.room.ExportHistory()
	name := "fiveleaf-roundtable"
	if slug := strings.Trim(markdownSlug(title), "-_"); slug != "" {
		slug, _ = agentVisibleText(slug, 200)
		name += "-" + slug
	}
	owner := w.a
	defaultPath := filepath.Join(owner.root, name+".md")
	fail := func(path string, err error) {
		message := locales.FailSave(filepath.Base(path), err.Error())
		if owner.meetingWindow == w && w.meeting == meeting && !w.creating {
			w.err = message
		}
		owner.fail("%s", message)
	}
	save := func(path string) {
		if path == "" {
			return
		}
		bg.Go(func() {
			dir := filepath.Dir(path)
			if resolved, err := filepath.EvalSymlinks(dir); err == nil {
				dir = resolved
			}
			err := streamPrivateExport(path, func(output io.Writer) error {
				writer := bufio.NewWriter(output)
				if _, err := writer.WriteString(heading); err != nil {
					return err
				}
				if err := history.Visit(func(message roundtable.Message) error {
					return writeMeetingMessage(writer, message, project, dir)
				}); err != nil {
					return err
				}
				return writer.Flush()
			})
			owner.post(func() {
				if err != nil {
					fail(path, err)
					return
				}
				owner.tell(note{message: locales.AgentChatExported(filepath.Base(path))})
			})
		})
	}
	owner.host.exportAs(defaultPath, "md", save)
}

var exportLabelEscapes = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "#", `\#`)

func exportLabel(label string) string {
	return exportLabelEscapes.Replace(strings.Join(strings.Fields(label), " "))
}

func writeMeetingMessage(writer *bufio.Writer, message roundtable.Message, project, dir string) error {
	author := cmp.Or(message.Name, message.AgentID, locales.AgentTitle())
	switch message.Role {
	case "user":
		author = locales.RoundtableHuman()
	case "assistant":
	case "task":
		if message.Task == nil {
			return nil
		}
	default:
		return nil
	}
	fmt.Fprintf(writer, "\n## %s\n\n%s", exportLabel(author), message.Time.Format(time.RFC3339Nano))
	if message.HumanConclusion {
		fmt.Fprintf(writer, " · %s", locales.RoundtableConclusion())
	}
	if task := message.Task; task != nil {
		if !task.Finished.IsZero() {
			fmt.Fprintf(writer, " → %s", task.Finished.Format(time.RFC3339Nano))
		}
		stage := locales.MenuTasks()
		switch task.Stage {
		case roundtable.Conclusion:
			stage = locales.RoundtableConclusion()
		case roundtable.Direct:
			stage = locales.RoundtableMemberReply()
		}
		fmt.Fprintf(writer, " · %s · %s · %s", locales.RoundtableHop(message.Hop), stage, taskStatusLabel(task.Status))
		if task.Assignment != "" {
			assignment := rebaseMeetingImages(task.Assignment, project, dir)
			fmt.Fprintf(writer, "\n\n> %s", strings.ReplaceAll(assignment, "\n", "\n> "))
		}
		if task.NoAnswer {
			fmt.Fprintf(writer, "\n\n%s", locales.RoundtableToolOnly())
		}
	}
	if message.Text != "" {
		writer.WriteString("\n\n")
		body := message.Text
		if message.Task != nil && message.Task.Status == roundtable.Failed {
			body = exportToolText(body)
		}
		writer.WriteString(rebaseMeetingImages(body, project, dir))
	}
	if task := message.Task; task != nil && len(task.Tools) > 0 {
		fmt.Fprintf(writer, "\n\n### %s\n", locales.AgentTools())
		for _, tool := range task.Tools {
			title := cmp.Or(tool.Title, tool.Kind, locales.AgentTools())
			fmt.Fprintf(writer, "\n- %s · %s\n", exportLabel(exportToolText(title)), agentStatusLabel(tool.Status))
			if tool.Detail != "" {
				fmt.Fprintf(writer, "\n      %s\n", strings.ReplaceAll(exportToolText(tool.Detail), "\n", "\n      "))
			}
			if tool.Note != "" {
				fmt.Fprintf(writer, "\n  %s\n\n      %s\n", locales.AlertNote(), strings.ReplaceAll(exportToolText(tool.Note), "\n", "\n      "))
			}
		}
	}
	return writer.WriteByte('\n')
}

var exportPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY(?: BLOCK)?-----.*?(?:-----END [A-Z ]*PRIVATE KEY(?: BLOCK)?-----|\z)`)
var exportCredentialLine = regexp.MustCompile(`(?im)^.*(?:\b(?:authorization|cookie|set-cookie)\s*:|\bbearer\s+\S+|(?:api[_-]?key|token|secret|password|passwd)["']?\s*[:=]\s*\S|--(?:password|token|api-key)\s+\S|://[^\s/@:]+:[^\s/@]+@|\b(?:sk-[a-z\d_-]{20,}|gh[pousr]_[a-z\d]{36,}|github_pat_[a-z\d_]{22,}|AKIA[0-9A-Z]{16})).*$`)

func exportToolText(value string) string {
	value = exportPrivateKey.ReplaceAllLiteralString(value, "[…]")
	return exportCredentialLine.ReplaceAllLiteralString(value, "[…]")
}

func rebaseMeetingImages(markdown, project, dir string) string {
	if project == "" || dir == project || !strings.Contains(markdown, "![") {
		return markdown
	}
	source := []byte(markdown + "\n")
	document := markdownParser(false).Parse(text.NewReader(source))
	type replacement struct {
		length int
		url    string
	}
	destinations := map[*byte]replacement{}
	ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		image, ok := node.(*ast.Image)
		if entering && ok && len(image.Destination) > 0 {
			value := util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(image.Destination)))
			dest, err := url.Parse(string(value))
			if err == nil && dest.Scheme == "" && dest.Host == "" && !strings.HasPrefix(string(value), "//") && filepath.IsLocal(filepath.FromSlash(dest.Path)) {
				path := filepath.Join(project, filepath.FromSlash(dest.Path))
				if relative, err := filepath.Rel(dir, path); err == nil {
					dest.Path, dest.RawPath = filepath.ToSlash(relative), ""
					destinations[&image.Destination[0]] = replacement{len(image.Destination), dest.String()}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if len(destinations) == 0 {
		return markdown
	}
	var output strings.Builder
	start := 0
	for offset := 0; offset < len(markdown); offset++ {
		if dest, found := destinations[&source[offset]]; found {
			output.WriteString(markdown[start:offset])
			output.WriteString(dest.url)
			start = offset + dest.length
			offset = start - 1
		}
	}
	output.WriteString(markdown[start:])
	return output.String()
}
