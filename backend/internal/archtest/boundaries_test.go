package archtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

var services = []string{"auth", "chat", "media", "presence", "gateway"}

// Инфраструктурные пакеты, которые прикладной и доменный код получает только
// через порты. Прямой импорт означал бы обход контракта.
var forbiddenImports = map[string]string{
	"database/sql": "database access",
	"net":          "network access",
	"net/http":     "network access",
	"os":           "filesystem access",
	"os/exec":      "process execution",
	"syscall":      "system calls",
}

func TestServicePackagesReachInfrastructureThroughPorts(t *testing.T) {
	t.Parallel()

	for _, service := range services {
		for _, file := range goFiles(t, filepath.Join("..", service)) {
			for _, imported := range imports(t, file) {
				if reason, blocked := forbiddenImports[imported]; blocked {
					t.Errorf("%s imports %q (%s); infrastructure must be reached through a port", file, imported, reason)
				}
			}
		}
	}
}

func TestCommandEntryPointsWireModules(t *testing.T) {
	t.Parallel()

	for _, service := range services {
		file := filepath.Join("..", "..", "cmd", service, "main.go")
		if !wiresModule(t, file, service) {
			t.Errorf("%s does not construct the %s module", file, service)
		}
	}
}

func TestCommandsListenWithTheSharedBootstrap(t *testing.T) {
	t.Parallel()

	for _, service := range services {
		file := filepath.Join("..", "..", "cmd", service, "main.go")
		found := false
		for _, imported := range imports(t, file) {
			if imported == "github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not use the shared bootstrap", file)
		}
	}
}

func goFiles(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	if len(files) == 0 {
		t.Fatalf("no go files in %s", dir)
	}
	var production []string
	for _, file := range files {
		if !strings.HasSuffix(file, "_test.go") {
			production = append(production, file)
		}
	}
	return production
}

func imports(t *testing.T, file string) []string {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var paths []string
	for _, imported := range parsed.Imports {
		paths = append(paths, strings.Trim(imported.Path.Value, `"`))
	}
	return paths
}

func wiresModule(t *testing.T, file, service string) bool {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	packageName := ""
	for _, imported := range parsed.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if strings.HasSuffix(path, "/internal/"+service) {
			packageName = service
			if imported.Name != nil {
				packageName = imported.Name.Name
			}
		}
	}
	if packageName == "" {
		return false
	}

	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if identifier.Name == packageName && selector.Sel.Name == "New" {
			found = true
		}
		return true
	})
	return found
}
