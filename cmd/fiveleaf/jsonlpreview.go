package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"syscall"
	"unicode/utf8"

	"fiveleaf/internal/bg"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const jsonlPageBytes = 256 << 10

type jsonlPage struct {
	number, last int64
	text         string
}

type jsonlPreview struct {
	path    string
	page    jsonlPage
	chunks  []string
	list    ui.ListState
	loading bool
	err     error
}

func readJSONLPage(path string, number int64) (jsonlPage, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return jsonlPage{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return jsonlPage{}, err
	}
	if !info.Mode().IsRegular() {
		return jsonlPage{}, os.ErrInvalid
	}
	last := max(0, info.Size()-1) / jsonlPageBytes
	number = max(0, min(number, last))
	offset := number * jsonlPageBytes
	data := make([]byte, min(info.Size()-offset, jsonlPageBytes+utf8.UTFMax-1))
	count, err := file.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return jsonlPage{}, err
	}
	data = data[:count]
	start, end := 0, min(count, jsonlPageBytes)
	if offset > 0 {
		for start < min(count, utf8.UTFMax-1) && !utf8.RuneStart(data[start]) {
			start++
		}
	}
	for end < count && !utf8.RuneStart(data[end]) {
		end++
	}
	data = data[start:end]
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return jsonlPage{}, errBinary
	}
	return jsonlPage{number: number, last: last, text: string(data)}, nil
}

func (a *app) loadJSONLPage(ed *editor, number int64) {
	preview, path := ed.jsonl, ed.path
	if preview.loading {
		return
	}
	preview.loading = true
	bg.Go(func() {
		page, err := readJSONLPage(path, number)
		chunks := previewTextChunks(page.text)
		a.post(func() {
			preview.loading, preview.err = false, err
			if err == nil {
				preview.page, preview.chunks, preview.list = page, chunks, ui.ListState{}
			}
		})
	})
}

func (a *app) jsonlPreviewView(c *ui.Context, ed *editor) {
	if ed.jsonl == nil || ed.jsonl.path != ed.path {
		ed.jsonl = &jsonlPreview{path: ed.path}
		a.loadJSONLPage(ed, 0)
	}
	preview := ed.jsonl
	page := preview.page
	palette, metrics := a.pal, a.pal.metricsOr()
	ui.Column(c.Key(ed)).Fill().MinWidth(0).MinHeight(0).Children(func() {
		ui.Row(c.Key("pages")).FillWidth().MinWidth(0).Padding(metrics.gap, metrics.pad).AlignItems(ui.Center).Children(func() {
			label := locales.JsonlPage(strconv.FormatInt(page.number+1, 10), strconv.FormatInt(page.last+1, 10))
			ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().FontSize(metrics.textSmall).TextColor(palette.muted)
			goTo := func(icon *ui.SVG, label string, target int64, disabled bool) {
				if a.iconButton(c.Key(label), icon, label, false).Disabled(preview.loading || disabled).Clicked() {
					a.loadJSONLPage(ed, target)
				}
			}
			goTo(iconChevronsLeft, locales.FirstPage(), 0, page.number == 0)
			goTo(iconChevronLeft, locales.PreviousPage(), page.number-1, page.number == 0)
			goTo(iconChevronRight, locales.NextPage(), page.number+1, page.number >= page.last)
			goTo(iconChevronsRight, locales.LastPage(), page.last, page.number >= page.last)
			goTo(iconRefresh, locales.Refresh(), page.number, false)
		})
		if preview.err != nil {
			message := locales.FailOpen(ed.name(), preview.err.Error())
			if errors.Is(preview.err, errBinary) {
				message = locales.FailBinary(ed.name())
			}
			ui.Text(c, message).FontSize(metrics.textBody).TextColor(palette.dangerFg).Padding(metrics.inset)
			return
		}
		if preview.loading && len(preview.chunks) == 0 {
			ui.Text(c, locales.RemoteReading(ed.name())).Padding(metrics.inset).TextColor(palette.muted)
			return
		}
		ui.List(c.Key("records").Key(page.number), &preview.list, len(preview.chunks), func(index int) {
			ui.Text(c, preview.chunks[index]).FillWidth().MinWidth(0).Font(a.settings.FontFamily).FontSize(a.settings.FontSize).TextColor(palette.text)
		}).Fill().MinHeight(0).Padding(metrics.gap, metrics.inset).Selectable()
	})
}
