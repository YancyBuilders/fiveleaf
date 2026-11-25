package plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func checkToolboxPage(dir string, manifest *Manifest) error {
	if manifest.Contributes.Toolbox == nil || manifest.Contributes.Toolbox.Builtin == "devtool" {
		return nil
	}
	file, err := manifest.Contributes.Toolbox.OpenAsset(dir, "")
	if err != nil {
		return fmt.Errorf("toolbox page %q: %w", manifest.Contributes.Toolbox.Page, err)
	}
	return file.Close()
}

func (p *Plugin) DevToolPage() string {
	if p.LegacyToolbox != "" {
		return p.LegacyToolbox
	}
	tool := p.Contributes.Toolbox
	if tool == nil {
		return ""
	}
	if tool.Builtin == "devtool" {
		return tool.ID
	}
	if p.Version == "1.1.0" && p.reg != nil {
		origin := p.reg.Origin(p.Name)
		const firstToolRelease = "161648266c015e7e1a1bb46271188cb372627270"
		toolName := tool.ID
		if toolName == "time" {
			toolName = "timestamp"
		}
		if !origin.Linked && origin.FromGit && origin.Commit == firstToolRelease && p.Name == "toolbox-"+toolName &&
			sourceIdentity(origin.URL) == "https://github.com/YancyBuilders/fiveleaf-plugins#plugins/tools/"+p.Name {
			return tool.ID
		}
	}
	return ""
}

func (tool *ToolboxContrib) OpenAsset(dir, name string) (*os.File, error) {
	if name == "" {
		name = path.Base(tool.Page)
	}
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") {
		return nil, fs.ErrInvalid
	}
	for component := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(component, ".") || strings.EqualFold(component, ManifestFile) {
			return nil, fs.ErrPermission
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	pageRoot, err := root.OpenRoot(filepath.FromSlash(path.Dir(tool.Page)))
	if err != nil {
		return nil, err
	}
	defer pageRoot.Close()
	asset := filepath.FromSlash(name)
	info, err := pageRoot.Stat(asset)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrInvalid
	}
	return pageRoot.Open(asset)
}
