package main

import (
	"cmp"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fiveleaf/internal/bg"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

func (message *agentMessage) exportable() bool {
	return message != nil && (message.role == "user" || message.role == "assistant" || message.role == "commentary") &&
		strings.TrimSpace(message.text) != ""
}

func (a *app) chatExportRow(c *ui.Context, chat *agentChat) {
	label := locales.AgentChatExport()
	enabled := slices.ContainsFunc(chat.messages, (*agentMessage).exportable)
	row := a.chatDetailRow(c.Key("export-chat"), label).Disabled(!enabled)
	ink := a.pal.text
	if !enabled {
		ink = a.pal.muted
	}
	row.Children(func() {
		ui.Icon(c, iconDownload).FontSize(15).TextColor(ink).Shrink(0)
		ui.Text(c, label).FontSize(a.pal.metricsOr().textBody).TextColor(ink).SingleLine().MinWidth(0)
	})
	if row.Clicked() {
		a.exportChat(chat)
	}
}

func (a *app) exportChat(chat *agentChat) {
	title := cmp.Or(chat.transcriptTitle(), locales.AgentThreadTitle())
	heading := "# " + title + "\n"
	if chat.continuation != nil && chat.continuation.truncated {
		heading += "\n> " + locales.SessionsContinuePartial() + "\n"
	}
	parts := []io.Reader{strings.NewReader(heading)}
	user, assistant := locales.AgentYou(), cmp.Or(chat.roleLabel(), locales.AgentTitle())
	for _, message := range chat.messages {
		if !message.exportable() {
			continue
		}
		author := assistant
		if message.role == "user" {
			author = user
		}
		parts = append(parts, strings.NewReader("\n## "+author+"\n\n"), strings.NewReader(message.text), strings.NewReader("\n"))
	}
	if len(parts) == 1 {
		return
	}
	name := "fiveleaf"
	if slug := strings.Trim(markdownSlug(title), "-_"); slug != "" {
		name += "-" + slug
	}
	a.host.exportAs(filepath.Join(a.root, name+".md"), "md", func(path string) {
		if path == "" {
			return
		}
		bg.Go(func() {
			err := writePrivateExport(path, io.MultiReader(parts...))
			a.post(func() {
				if err != nil {
					a.fail("%s", locales.FailSave(filepath.Base(path), err.Error()))
					return
				}
				a.tell(note{message: locales.AgentChatExported(filepath.Base(path))})
			})
		})
	})
}

func writePrivateExport(path string, contents io.Reader) error {
	return streamPrivateExport(path, func(output io.Writer) error {
		_, err := io.Copy(output, contents)
		return err
	})
}

func streamPrivateExport(path string, write func(io.Writer) error) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".fiveleaf-export-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = write(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), path)
	}
	return err
}
