package plugin

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"fiveleaf/internal/keymap"
)

const (
	maxIconAssetBytes = 1 << 20
	maxIconsPerPack   = 2048
	maxIconPackBytes  = 32 << 20
)

type appearanceAssets struct {
	files fs.FS
	sizes map[string]int64
}

func checkAppearanceAssets(dir string, manifest *Manifest) error {
	contributions := &manifest.Contributes
	if len(contributions.Themes) == 0 && len(contributions.IconThemes) == 0 {
		return nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	assets := appearanceAssets{files: root.FS(), sizes: map[string]int64{}}
	palettes := map[string]bool{}
	for _, theme := range contributions.Themes {
		for _, name := range []string{theme.Path, theme.DarkPath} {
			if name == "" || palettes[name] {
				continue
			}
			if err := assets.checkColorTheme(name); err != nil {
				return fmt.Errorf("theme %q (%s): %w", theme.ID, name, err)
			}
			palettes[name] = true
		}
	}
	packs := map[string]bool{}
	for _, theme := range contributions.IconThemes {
		if packs[theme.Path] {
			continue
		}
		if err := assets.checkIconPack(theme.Path); err != nil {
			return fmt.Errorf("icon theme %q (%s): %w", theme.ID, theme.Path, err)
		}
		packs[theme.Path] = true
	}
	return nil
}

func (assets appearanceAssets) openRegular(name string) (fs.File, int64, error) {
	info, err := fs.Stat(assets.files, name)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s must be a regular file", name)
	}
	file, err := assets.files.Open(name)
	return file, info.Size(), err
}

func (assets appearanceAssets) checkColorTheme(name string) error {
	file, _, err := assets.openRegular(name)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	return DecodeColorTheme(data, &struct{}{})
}

func DecodeColorTheme(data []byte, target any) error {
	data = bytes.TrimSpace(keymap.StripJSONC(data))
	if len(data) == 0 || data[0] != '{' {
		return fmt.Errorf("expected a color theme object")
	}
	return json.Unmarshal(data, target)
}

func (assets appearanceAssets) svgSize(name string) (int64, error) {
	if size, ok := assets.sizes[name]; ok {
		return size, nil
	}
	file, size, err := assets.openRegular(name)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if size > maxIconAssetBytes {
		return 0, fmt.Errorf("%s exceeds the 1 MiB icon limit", name)
	}
	if err := ValidateIconSVG(io.LimitReader(file, maxIconAssetBytes+1)); err != nil {
		return 0, err
	}
	assets.sizes[name] = size
	return size, nil
}

func ValidateIconSVG(reader io.Reader) error {
	decoder := xml.NewDecoder(reader)
	depth, root := 0, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if root || token.Name.Local != "svg" {
					return fmt.Errorf("expected one SVG root element")
				}
				root = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(token)) != 0 {
				return fmt.Errorf("unexpected text outside SVG")
			}
		}
	}
	if !root || depth != 0 {
		return fmt.Errorf("incomplete SVG document")
	}
	return nil
}

func (assets appearanceAssets) checkIconPack(name string) error {
	file, _, err := assets.openRegular(name)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIconAssetBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxIconAssetBytes {
		return fmt.Errorf("%s exceeds the 1 MiB icon limit", name)
	}
	var pack struct {
		Icons map[string]string `json:"icons"`
	}
	if err := json.Unmarshal(data, &pack); err != nil {
		return err
	}
	if len(pack.Icons) == 0 || len(pack.Icons) > maxIconsPerPack {
		return fmt.Errorf("expected from 1 to %d icons", maxIconsPerPack)
	}
	dir := path.Dir(name)
	var total int64
	for id, ref := range pack.Icons {
		if id == "" || !fs.ValidPath(ref) || strings.ContainsAny(ref, "\\:\x00") || !strings.EqualFold(path.Ext(ref), ".svg") {
			return fmt.Errorf("invalid SVG path %q for icon %q", ref, id)
		}
		asset := path.Join(dir, ref)
		size, err := assets.svgSize(asset)
		if err != nil {
			return fmt.Errorf("%s: %w", asset, err)
		}
		total += size
		if total > maxIconPackBytes {
			return fmt.Errorf("SVG assets exceed %d bytes", maxIconPackBytes)
		}
	}
	return nil
}
