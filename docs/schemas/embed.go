package schemas

import "embed"

// FS contains only the shared and per-method response schemas needed by the runner.
//
//go:embed shared.json *.response.json
var FS embed.FS
