package bundled

import "embed"

//go:embed toolbox-*/plugin.json
var Tools embed.FS
