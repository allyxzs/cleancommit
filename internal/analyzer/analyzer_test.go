package analyzer

import (
	"strings"
	"testing"

	"cleancommit/internal/config"
	"cleancommit/internal/git"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{"*.log", "a/b/c.log", true},
		{"*.log", "a/b/c.logx", false},
		{"node_modules/", "web/node_modules/x/y.js", true},
		{"node_modules/", "src/node_modules_x/y.js", false},
		{"docs/legacy/**", "docs/legacy/a/b.md", true},
		{"docs/legacy/**", "other/docs/legacy/a.md", false},
		{"src/**/*.go", "src/a.go", true},
		{"src/**/*.go", "src/x/y/a.go", true},
		{".env.*", ".env.local", true},
		{".env", ".env", true},
		{".env", "sub/.env", true},
		{"*.egg-info/", "pkg/foo.egg-info/PKG-INFO", true},
	}
	for _, c := range cases {
		re, err := GlobToRegexp(c.pat)
		if err != nil {
			t.Fatalf("%q: %v", c.pat, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q vs %q = %v, esperado %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestBuiltinRulesCompile(t *testing.T) {
	// MustCompile entra em pânico se algum padrão embutido for inválido.
	if len(builtinLineRules()) == 0 {
		t.Fatal("sem regras embutidas")
	}
	if _, err := builtinFileRules(); err != nil {
		t.Fatal(err)
	}
	if len(Catalog()) == 0 {
		t.Fatal("catálogo vazio")
	}
}

func hasRule(res *Result, rule string) bool {
	for _, f := range res.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func newAnalyzer(t *testing.T) *Analyzer {
	t.Helper()
	an, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	return an
}

func TestDetectsDebugAndFiles(t *testing.T) {
	files := []*git.FileChange{
		{
			Path: "src/app.js", Status: git.StatusModified,
			Added: []git.AddedLine{
				{No: 11, Text: `  console.log("x");`},
				{No: 12, Text: "  debugger;"},
			},
		},
		{Path: ".env", Status: git.StatusAdded, Added: []git.AddedLine{{No: 1, Text: "A=1"}}},
		{Path: "node_modules/x/index.js", Status: git.StatusAdded},
	}
	res := newAnalyzer(t).Analyze(files)

	for _, rule := range []string{"js-console", "js-debugger", "env-file", "dependency-dir"} {
		if !hasRule(res, rule) {
			t.Errorf("regra %q deveria ter disparado: %+v", rule, res.Findings)
		}
	}
	if !res.Blocked("error") {
		t.Error("deveria bloquear com fail_on=error")
	}
	if res.Blocked("never") {
		t.Error("fail_on=never nunca bloqueia")
	}
}

func TestIgnoreComment(t *testing.T) {
	files := []*git.FileChange{{
		Path: "a.js", Status: git.StatusModified,
		Added: []git.AddedLine{
			{No: 1, Text: "console.log(1) // cleancommit:ignore"},
			{No: 2, Text: "console.log(2) // cleancommit:ignore=outra-regra"},
			{No: 3, Text: "console.log(3) // cleancommit:ignore=js-console"},
		},
	}}
	res := newAnalyzer(t).Analyze(files)
	n := 0
	for _, f := range res.Findings {
		if f.Rule == "js-console" {
			n++
			if f.Line != 2 {
				t.Errorf("só a linha 2 deveria disparar, veio a %d", f.Line)
			}
		}
	}
	if n != 1 {
		t.Errorf("esperava 1 achado js-console, veio %d", n)
	}
}

func TestSecretIsMasked(t *testing.T) {
	files := []*git.FileChange{{
		Path: "config.go", Status: git.StatusModified,
		Added: []git.AddedLine{{No: 1, Text: `key := "AKIAABCDEFGHIJKLMNOP"`}},
	}}
	res := newAnalyzer(t).Analyze(files)
	if !hasRule(res, "aws-access-key") {
		t.Fatalf("chave AWS deveria ser detectada: %+v", res.Findings)
	}
	for _, f := range res.Findings {
		if strings.Contains(f.Snippet, "AKIAABCDEFGHIJKLMNOP") {
			t.Errorf("o segredo não pode aparecer inteiro no snippet: %q", f.Snippet)
		}
	}
}

func TestScope(t *testing.T) {
	cfg := config.Default()
	cfg.Limits.MaxFiles = 2
	an, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	files := []*git.FileChange{
		{Path: "a.go", Status: git.StatusModified, Added: []git.AddedLine{{No: 1, Text: "x"}}},
		{Path: "b.go", Status: git.StatusModified, Added: []git.AddedLine{{No: 1, Text: "x"}}},
		{Path: "c.go", Status: git.StatusModified, Added: []git.AddedLine{{No: 1, Text: "x"}}},
		{Path: "package-lock.json", Status: git.StatusModified},
	}
	res := an.Analyze(files)
	if !hasRule(res, "big-commit-files") {
		t.Errorf("deveria apontar commit grande: %+v", res.Findings)
	}
	if !hasRule(res, "lockfile-only") {
		t.Errorf("deveria apontar lockfile sem manifesto: %+v", res.Findings)
	}
}

func TestCustomRulesAndDisable(t *testing.T) {
	cfg := config.Default()
	cfg.Rules = []config.CustomRule{{ID: "no-foo", Pattern: `foo\(`, Severity: "error", Paths: []string{"src/**"}, Message: "sem foo"}}
	cfg.ForbiddenFiles = []config.ForbiddenFile{{Pattern: "*.secret", Severity: "error"}}
	cfg.Disable = []string{"js-console"}
	an, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	files := []*git.FileChange{
		{Path: "src/a.js", Status: git.StatusModified, Added: []git.AddedLine{{No: 1, Text: "foo(1); console.log(2)"}}},
		{Path: "lib/b.js", Status: git.StatusModified, Added: []git.AddedLine{{No: 1, Text: "foo(1)"}}},
		{Path: "x/data.secret", Status: git.StatusAdded},
	}
	res := an.Analyze(files)
	if !hasRule(res, "no-foo") || !hasRule(res, "forbidden-file") {
		t.Errorf("regras personalizadas deveriam disparar: %+v", res.Findings)
	}
	if hasRule(res, "js-console") {
		t.Error("js-console foi desativada")
	}
	count := 0
	for _, f := range res.Findings {
		if f.Rule == "no-foo" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("no-foo só vale para src/**, achados: %d", count)
	}
}
