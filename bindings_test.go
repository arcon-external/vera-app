package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"vera-app/backend"
)

// frontend/wailsjs/go/** is generated build output, and nothing checked that it
// matched the Go API.
//
// It went stale by accident: #294 added ExportTo and RenderPDF as bound methods
// on Quotation and never regenerated the bindings, so the committed .js and .d.ts
// were missing two methods. Nothing failed. Nothing would have. The desktop calls
// these through the generated bindings, so a missing entry is a method the
// frontend cannot reach -- discovered at runtime by whoever clicks the button.
//
// AGENTS.md §9 states the rule ("if method signatures change, regenerate and
// verify frontend/wailsjs/go/*"), and a rule with no enforcement is how this
// happened. This is the enforcement, and it needs no Wails CLI: the generated
// files are text, and the Go side is reflection.

// boundObjects mirrors main.go's Bind list. A test rather than a shared
// constant because main.go's list is a slice literal inside a struct, and
// extracting it to make a test easier would be the tail wagging the dog.
//
// If a fifth object is bound, add it here: the test then covers it, and until
// someone does, the omission is visible in one place rather than invisible in
// four generated files.
func boundObjects() map[string]any {
	return map[string]any{
		"Quotation": &backend.Quotation{},
		"AuthAPI":   backend.NewAuthAPI(),
		"AdminAPI":  backend.NewAdminAPI(),
		"UpdateAPI": backend.NewUpdateAPI(),
	}
}

// generatedBindingsPath is where wails generate module writes, relative to the
// repository root this test runs from.
func generatedBindingsPath(t *testing.T, typeName string) string {
	t.Helper()
	path := filepath.Join("frontend", "wailsjs", "go", "backend", typeName+".js")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cannot read %s: %v\n"+
			"If the bindings were never generated, run: wails generate module", path, err)
	}
	return path
}

// TestGeneratedBindingsCoverEveryBoundMethod is the guard.
func TestGeneratedBindingsCoverEveryBoundMethod(t *testing.T) {
	for typeName, object := range boundObjects() {
		t.Run(typeName, func(t *testing.T) {
			raw, err := os.ReadFile(generatedBindingsPath(t, typeName))
			if err != nil {
				t.Fatal(err)
			}
			generated := string(raw)

			methods := exportedMethods(object)
			// A type with no exported methods would make this test vacuous, and
			// that is exactly what a rename to an unexported type looks like.
			if len(methods) == 0 {
				t.Fatalf("%s has no exported methods, so this check proves nothing", typeName)
			}

			var missing []string
			for _, method := range methods {
				// The generated file calls through a literal, so looking for the
				// method's own name inside that call is enough to tell whether it
				// was generated -- and it will not match a comment.
				if !strings.Contains(generated, "['"+method+"']") {
					missing = append(missing, method)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				t.Errorf("%s is bound in Go but %d of its methods are absent from %s:\n  %s\n\n"+
					"Run: wails generate module\n"+
					"and commit frontend/wailsjs/go/. The desktop calls these through the "+
					"generated bindings, so a missing one is a method the frontend cannot reach.",
					typeName, len(missing), generatedBindingsPath(t, typeName),
					strings.Join(missing, "\n  "))
			}
		})
	}
}

// TestGeneratedBindingsHaveNoOrphans is the other direction.
//
// A method deleted in Go but left in the generated file is a frontend that
// compiles and then fails at the call, which is worse than one that fails to
// build. The .d.ts is what the TypeScript side checks against, so an orphan
// there is a promise the Go side no longer keeps.
func TestGeneratedBindingsHaveNoOrphans(t *testing.T) {
	for typeName, object := range boundObjects() {
		t.Run(typeName, func(t *testing.T) {
			raw, err := os.ReadFile(generatedBindingsPath(t, typeName))
			if err != nil {
				t.Fatal(err)
			}
			declared := exportedMethods(object)

			for _, generated := range generatedMethodNames(string(raw)) {
				if !contains(declared, generated) {
					t.Errorf("%s.%s exists in %s but not in Go; the bindings are stale.\n"+
						"Run: wails generate module",
						typeName, generated, generatedBindingsPath(t, typeName))
				}
			}
		})
	}
}

func exportedMethods(object any) []string {
	typ := reflect.TypeOf(object)
	var methods []string
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		// Only methods the frontend could call: Wails binds exported methods
		// with no context argument, but the context form is not used here, so
		// filtering on export is enough and filtering further would silently
		// exclude something.
		if method.PkgPath == "" {
			methods = append(methods, method.Name)
		}
	}
	return methods
}

// generatedMethodNames pulls the method names out of a generated binding file.
//
// The generated file calls through window['go']['backend']['Quotation']['Method'](),
// so the method segment is the one followed by a call. Matching that shape keeps
// the package and type segments out without special-casing them.
var generatedMethodCall = regexp.MustCompile(`\['([A-Za-z_][A-Za-z0-9_]*)'\]\(`)

func generatedMethodNames(generated string) []string {
	seen := map[string]bool{}
	var names []string
	for _, match := range generatedMethodCall.FindAllStringSubmatch(generated, -1) {
		name := match[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func contains(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if candidate == needle {
			return true
		}
	}
	return false
}
