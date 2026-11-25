// Package suggest sugere mensagens de commit no padrão Conventional Commits.
package suggest

import (
	"fmt"
	pathpkg "path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"cleancommit/internal/classify"
	"cleancommit/internal/git"
)

const maxHeader = 72

// Suggestion é uma mensagem de commit sugerida.
type Suggestion struct {
	Type         string
	Scope        string
	Subject      string
	Body         string
	Footer       string
	Alternatives []string // outros tipos plausíveis
}

// HeaderFor monta a primeira linha usando o tipo informado.
func (s *Suggestion) HeaderFor(t string) string {
	if s.Scope != "" {
		return fmt.Sprintf("%s(%s): %s", t, s.Scope, s.Subject)
	}
	return fmt.Sprintf("%s: %s", t, s.Subject)
}

// Header é a primeira linha da mensagem.
func (s *Suggestion) Header() string {
	return s.HeaderFor(s.Type)
}

// Message devolve a mensagem completa (cabeçalho, corpo e rodapé).
func (s *Suggestion) Message() string {
	msg := s.Header()
	if s.Body != "" {
		msg += "\n\n" + s.Body
	}
	if s.Footer != "" {
		msg += "\n\n" + s.Footer
	}
	return msg
}

type vocab struct {
	add, remove, rename, update, fix, refactor string
	docs, tests, deps, ci, format              string
	files, in, of, and, more                   string
}

var vocabs = map[string]vocab{
	"pt": {
		add: "adiciona", remove: "remove", rename: "renomeia", update: "atualiza",
		fix: "corrige", refactor: "refatora",
		docs: "documentação", tests: "testes", deps: "dependências", ci: "pipeline de CI",
		format: "ajusta formatação",
		files:  "arquivos", in: "em", of: "de", and: "e", more: "- … e mais %d arquivos",
	},
	"en": {
		add: "add", remove: "remove", rename: "rename", update: "update",
		fix: "fix", refactor: "refactor",
		docs: "documentation", tests: "tests", deps: "dependencies", ci: "CI pipeline",
		format: "adjust formatting",
		files:  "files", in: "in", of: "for", and: "and", more: "- … and %d more files",
	},
}

var (
	fixRe  = regexp.MustCompile(`(?i)\b(fix|bug|hotfix|corrig\w*|erro|error|panic|crash|null|nil|npe)\b`)
	jiraRe = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)
	numRe  = regexp.MustCompile(`(?:^|[/_-])(\d{2,})(?:[/_-]|$)`)

	genericNames = map[string]bool{
		"index": true, "main": true, "mod": true, "lib": true, "init": true, "__init__": true,
	}
	branchTypes = map[string]string{
		"feat": "feat", "feature": "feat", "fix": "fix", "bugfix": "fix", "hotfix": "fix",
		"chore": "chore", "docs": "docs", "doc": "docs", "refactor": "refactor",
		"test": "test", "tests": "test", "perf": "perf", "ci": "ci", "build": "build", "style": "style",
	}
)

// Build gera uma sugestão a partir das alterações. Devolve nil sem alterações.
func Build(files []*git.FileChange, branch, lang string) *Suggestion {
	if len(files) == 0 {
		return nil
	}
	v, ok := vocabs[lang]
	if !ok {
		v = vocabs["pt"]
	}
	rel := relevant(files)

	counts := map[classify.Kind]int{}
	for _, f := range rel {
		counts[classify.Of(f.Path)]++
	}
	only := func(k classify.Kind) bool { return counts[k] == len(rel) }

	typ, scopeOverride := "", ""
	switch {
	case only(classify.Docs):
		typ = "docs"
	case only(classify.Test):
		typ = "test"
	case only(classify.CI):
		typ = "ci"
	case only(classify.Deps):
		typ, scopeOverride = "build", "deps"
	case only(classify.Generated):
		typ, scopeOverride = "chore", "deps"
	case only(classify.Build):
		typ = "build"
	case only(classify.Config):
		typ = "chore"
	case allWhitespace(rel):
		typ = "style"
	}
	if typ == "" {
		typ = codeType(rel, branch)
	}

	scope := scopeOverride
	if scope == "" && typ != "docs" && typ != "ci" {
		scope = decideScope(rel)
	}

	s := &Suggestion{
		Type:    typ,
		Scope:   scope,
		Subject: buildSubject(v, typ, scope, rel),
		Body:    buildBody(v, rel),
		Footer:  ticket(branch),
	}

	if over := utf8.RuneCountInString(s.Header()) - maxHeader; over > 0 {
		r := []rune(s.Subject)
		keep := len(r) - over - 1
		if keep < 10 {
			keep = 10
		}
		if keep < len(r) {
			s.Subject = string(r[:keep]) + "…"
		}
	}

	switch typ {
	case "feat", "fix", "refactor":
		for _, t := range []string{"feat", "fix", "refactor"} {
			if t != typ {
				s.Alternatives = append(s.Alternatives, t)
			}
		}
	}
	return s
}

func relevant(files []*git.FileChange) []*git.FileChange {
	var out []*git.FileChange
	for _, f := range files {
		if classify.Of(f.Path) != classify.Generated {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return files
	}
	return out
}

func allWhitespace(rel []*git.FileChange) bool {
	for _, f := range rel {
		if !f.WhitespaceOnly {
			return false
		}
	}
	return true
}

func branchType(branch string) string {
	if i := strings.IndexAny(branch, "/-_"); i > 0 {
		if t, ok := branchTypes[strings.ToLower(branch[:i])]; ok {
			return t
		}
	}
	return ""
}

func codeType(rel []*git.FileChange, branch string) string {
	if t := branchType(branch); t != "" {
		return t
	}
	newCode, added, removed := 0, 0, 0
	keyword := false
	for _, f := range rel {
		added += len(f.Added)
		removed += f.Removed
		if classify.Of(f.Path) != classify.Code {
			continue
		}
		if f.Status == git.StatusAdded {
			newCode++
		}
		if !keyword {
			for _, l := range f.Added {
				if fixRe.MatchString(l.Text) {
					keyword = true
					break
				}
			}
		}
	}
	switch {
	case newCode > 0:
		return "feat"
	case keyword:
		return "fix"
	case removed > added*2:
		return "refactor"
	case added > removed:
		return "feat"
	}
	return "refactor"
}

func decideScope(rel []*git.FileChange) string {
	mods := map[string]bool{}
	for _, f := range rel {
		m := classify.Module(f.Path)
		if m == "" {
			return ""
		}
		mods[m] = true
	}
	if len(mods) != 1 {
		return ""
	}
	for m := range mods {
		if i := strings.LastIndex(m, "/"); i >= 0 {
			m = m[i+1:]
		}
		return sanitizeScope(m)
	}
	return ""
}

func sanitizeScope(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

func statusVerb(v vocab, rel []*git.FileChange) string {
	st := rel[0].Status
	for _, f := range rel {
		if f.Status != st {
			return v.update
		}
	}
	switch st {
	case git.StatusAdded:
		return v.add
	case git.StatusDeleted:
		return v.remove
	case git.StatusRenamed:
		return v.rename
	}
	return v.update
}

func buildSubject(v vocab, typ, scope string, rel []*git.FileChange) string {
	names := objectNames(rel)
	obj := describe(v, names, scope, len(rel))
	verb := statusVerb(v, rel)

	if scope == "deps" && (typ == "build" || typ == "chore") {
		return v.update + " " + v.deps
	}
	switch typ {
	case "docs":
		if len(names) == 1 {
			return v.update + " " + names[0]
		}
		return v.update + " " + v.docs
	case "test":
		return verb + " " + v.tests + " " + v.of + " " + obj
	case "ci":
		return v.update + " " + v.ci
	case "style":
		return v.format + " " + v.in + " " + obj
	case "fix":
		return v.fix + " " + obj
	case "refactor":
		return v.refactor + " " + obj
	}
	return verb + " " + obj
}

func describe(v vocab, names []string, scope string, n int) string {
	switch {
	case len(names) == 1:
		return names[0]
	case len(names) == 2 || len(names) == 3:
		head := strings.Join(names[:len(names)-1], ", ")
		return head + " " + v.and + " " + names[len(names)-1]
	}
	if scope != "" && scope != "deps" {
		return fmt.Sprintf("%d %s %s %s", n, v.files, v.in, scope)
	}
	return fmt.Sprintf("%d %s", n, v.files)
}

func objectNames(rel []*git.FileChange) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range rel {
		n := humanName(f.Path)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func humanName(p string) string {
	base := pathpkg.Base(p)
	name := strings.TrimSuffix(base, pathpkg.Ext(base))
	if name == "" {
		name = strings.TrimPrefix(base, ".")
	}
	for _, suf := range []string{".test", ".spec", "_test", "_spec"} {
		name = strings.TrimSuffix(name, suf)
	}
	name = strings.TrimPrefix(name, "test_")
	if genericNames[strings.ToLower(name)] {
		dir := pathpkg.Base(pathpkg.Dir(p))
		if dir != "." && dir != "/" && dir != "" {
			name = dir
		}
	}
	name = splitCamel(name)
	name = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(name)
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

func splitCamel(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(rs[i-1]) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func buildBody(v vocab, rel []*git.FileChange) string {
	if len(rel) < 2 {
		return ""
	}
	files := append([]*git.FileChange(nil), rel...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Churn() > files[j].Churn() })

	const limit = 8
	var lines []string
	for i, f := range files {
		if i >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("- %s (+%d -%d)", f.Path, len(f.Added), f.Removed))
	}
	if len(files) > limit {
		lines = append(lines, fmt.Sprintf(v.more, len(files)-limit))
	}
	return strings.Join(lines, "\n")
}

func ticket(branch string) string {
	if m := jiraRe.FindString(branch); m != "" {
		return "Refs: " + m
	}
	if m := numRe.FindStringSubmatch(branch); m != nil {
		return "Refs: #" + m[1]
	}
	return ""
}
