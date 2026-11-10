package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestStylesComeFromTheRegistry enforces gr's ADR 8 here too: only the look package creates styles or
// sets colors.
func TestStylesComeFromTheRegistry(t *testing.T) {
	// lipgloss functions that make styles and colors, and style methods that set colors.
	constructors := map[string]bool{"NewStyle": true, "Color": true, "LightDark": true}
	setters := map[string]bool{"Foreground": true, "Background": true, "BorderForeground": true, "BorderBackground": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, _ := sel.X.(*ast.Ident)
			if setters[sel.Sel.Name] || constructors[sel.Sel.Name] && pkg != nil && pkg.Name == "lipgloss" {
				t.Errorf("%s: %s outside the registry; add an element and a constructor to the look package", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}
