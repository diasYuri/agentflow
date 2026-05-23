package agentchannel_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentChannelDoesNotImportDeliveryAdapters(t *testing.T) {
	forbidden := map[string]string{
		"github.com/diasYuri/agentflow/internal/web":   "web adapter",
		"github.com/diasYuri/agentflow/internal/slack": "slack adapter",
		"net/http": "HTTP transport",
	}
	root := "."
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			for prefix, label := range forbidden {
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					t.Fatalf("agentchannel must not import %s: %s imports %s", label, path, importName(file, imported))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func importName(file *ast.File, imported *ast.ImportSpec) string {
	if imported.Name != nil {
		return imported.Name.Name
	}
	return strings.Trim(imported.Path.Value, `"`)
}
