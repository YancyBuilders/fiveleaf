package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	uiicons "github.com/smallnest/uix/components/lucide"

	"fiveleaf/internal/settings"
	"fiveleaf/locales"
)

func (w *prefsWindow) micaPresetControls(c *ui.Context) {
	compactActionRow(c.Key("mica-presets"), ui.Start).Children(func() {
		if actionButton(c.Key("import"), locales.PrefsMicaImport(), uiicons.Download()).Disabled(w.micaBusy).Clicked() {
			w.importMicaPreset()
		}
		if actionButton(c.Key("export"), locales.PrefsMicaExport(), uiicons.Upload()).Disabled(w.micaBusy).Clicked() {
			w.exportMicaPreset()
		}
	})
}

func (w *prefsWindow) importMicaPreset() {
	if w.micaBusy {
		return
	}
	w.micaBusy = true
	parent := w.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: locales.PrefsMicaImportTitle(), ButtonLabel: locales.DialogOpen(),
			Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
		})
		var path string
		var raw []byte
		if err == nil && len(paths) > 0 {
			path = paths[0]
			raw, err = readMicaPreset(path)
		}
		w.post(func() { w.finishMicaImport(path, raw, err) })
	}()
}

func readMicaPreset(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// The decoder rejects the extra byte if the file exceeds the limit.
	return io.ReadAll(io.LimitReader(f, settings.MaxMicaPresetSize+1))
}

func (w *prefsWindow) finishMicaImport(path string, raw []byte, err error) {
	w.micaBusy = false
	if err == nil && path == "" {
		return
	}
	if err == nil {
		p := w.m.s.Prefs
		err = p.ImportMica(raw)
		if err == nil {
			if micaParameter(w.held) || w.held == "windowOpacity" {
				w.held = ""
			}
			w.m.reload(p)
			w.m.s.Changed()
		}
	}
	if err != nil {
		message := locales.FailOpen(filepath.Base(path), err.Error())
		if path == "" {
			message = locales.FailWithError(locales.PrefsMicaImportTitle(), err.Error())
		}
		w.errors = append(w.errors, message)
		return
	}
	w.notes = append(w.notes, ui.Toast{ID: "mica-imported", Title: locales.PrefsMicaImported(filepath.Base(path))})
}

func (w *prefsWindow) exportMicaPreset() {
	if w.micaBusy {
		return
	}
	// Export the saved values rather than an unfinished slider preview.
	raw, err := w.m.s.Prefs.ExportMica()
	if err != nil {
		w.errors = append(w.errors, err.Error())
		return
	}
	w.micaBusy = true
	parent := w.win
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: locales.PrefsMicaExportTitle(), ButtonLabel: locales.DialogExport(),
			DefaultPath: "fiveleaf-mica.json", CreateDirectories: true,
			Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
		})
		if err == nil && path != "" {
			err = settings.WriteFile(path, raw)
		}
		w.post(func() { w.finishMicaExport(path, err) })
	}()
}

func (w *prefsWindow) finishMicaExport(path string, err error) {
	w.micaBusy = false
	if err != nil {
		message := locales.FailSave(filepath.Base(path), err.Error())
		if path == "" {
			message = locales.FailWithError(locales.PrefsMicaExportTitle(), err.Error())
		}
		w.errors = append(w.errors, message)
		return
	}
	if path != "" {
		w.notes = append(w.notes, ui.Toast{ID: "mica-exported", Title: locales.PrefsMicaExported(filepath.Base(path))})
	}
}
