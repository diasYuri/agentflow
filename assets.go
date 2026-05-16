package agentflow

import "embed"

// DesktopAssets contem os arquivos estaticos do frontend desktop (frontend/desktop/dist).
//
//go:embed all:frontend/desktop/dist
var DesktopAssets embed.FS
