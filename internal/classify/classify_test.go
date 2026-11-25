package classify

import "testing"

func TestOf(t *testing.T) {
	cases := map[string]Kind{
		"internal/auth/token.go":        Code,
		"internal/auth/token_test.go":   Test,
		"README.md":                     Docs,
		"docs/guia.html":                Docs,
		".github/workflows/ci.yml":      CI,
		"package.json":                  Deps,
		"package-lock.json":             Generated,
		"Makefile":                      Build,
		"Dockerfile":                    Build,
		"tsconfig.json":                 Config,
		".gitignore":                    Config,
		"assets/logo.png":               Asset,
		"web/app.min.js":                Generated,
		"src/components/Button.spec.ts": Test,
	}
	for p, want := range cases {
		if got := Of(p); got != want {
			t.Errorf("Of(%q) = %q, esperado %q", p, got, want)
		}
	}
}

func TestManifest(t *testing.T) {
	if m, ok := Manifest("web/yarn.lock"); !ok || m != "web/package.json" {
		t.Errorf("Manifest(web/yarn.lock) = %q, %v", m, ok)
	}
	if m, ok := Manifest("go.sum"); !ok || m != "go.mod" {
		t.Errorf("Manifest(go.sum) = %q, %v", m, ok)
	}
	if _, ok := Manifest("main.go"); ok {
		t.Errorf("main.go não é lockfile")
	}
}

func TestModule(t *testing.T) {
	cases := map[string]string{
		"main.go":                  "",
		"src/main.go":              "",
		"internal/auth/token.go":   "internal/auth",
		"web/components/button.js": "web",
	}
	for p, want := range cases {
		if got := Module(p); got != want {
			t.Errorf("Module(%q) = %q, esperado %q", p, got, want)
		}
	}
}
