package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cleancommit/internal/classify"
	"cleancommit/internal/config"
	"cleancommit/internal/git"
)

const (
	ignoreTag  = "cleancommit:ignore"
	maxLineLen = 2000
)

// Analyzer aplica regras embutidas e personalizadas às alterações.
type Analyzer struct {
	cfg       *config.Config
	lineRules []lineRule
	fileRules []fileRule
	ignore    []*regexp.Regexp
	allow     []*regexp.Regexp
	disabled  map[string]bool
}

// New prepara um analisador a partir da configuração.
func New(cfg *config.Config) (*Analyzer, error) {
	a := &Analyzer{cfg: cfg, disabled: map[string]bool{}}
	for _, id := range cfg.Disable {
		a.disabled[id] = true
	}

	var err error
	if a.ignore, err = compileGlobs(cfg.IgnorePaths); err != nil {
		return nil, fmt.Errorf("ignore_paths: %w", err)
	}
	if a.allow, err = compileGlobs(cfg.AllowFiles); err != nil {
		return nil, fmt.Errorf("allow_files: %w", err)
	}

	for _, r := range builtinLineRules() {
		if !a.disabled[r.ID] {
			a.lineRules = append(a.lineRules, r)
		}
	}
	if a.fileRules, err = builtinFileRules(); err != nil {
		return nil, err
	}

	for _, c := range cfg.Rules {
		if a.disabled[c.ID] {
			continue
		}
		re, err := regexp.Compile(c.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regra %q: %w", c.ID, err)
		}
		paths, err := compileGlobs(c.Paths)
		if err != nil {
			return nil, fmt.Errorf("regra %q: %w", c.ID, err)
		}
		sev, ok := ParseSeverity(c.Severity)
		if !ok {
			sev = Warning
		}
		msg := c.Message
		if msg == "" {
			msg = c.Description
		}
		if msg == "" {
			msg = fmt.Sprintf("Regra personalizada %q", c.ID)
		}
		a.lineRules = append(a.lineRules, lineRule{
			ID: c.ID, Category: "custom", Severity: sev, Re: re, Paths: paths, Message: msg,
		})
	}

	for _, ff := range cfg.ForbiddenFiles {
		re, err := GlobToRegexp(ff.Pattern)
		if err != nil {
			return nil, fmt.Errorf("forbidden_files %q: %w", ff.Pattern, err)
		}
		sev, ok := ParseSeverity(ff.Severity)
		if !ok {
			sev = Error
		}
		msg := ff.Message
		if msg == "" {
			msg = "Arquivo proibido pela configuração do projeto"
		}
		a.fileRules = append(a.fileRules, fileRule{ID: "forbidden-file", Severity: sev, re: re, Message: msg})
	}
	return a, nil
}

// Analyze executa todas as regras sobre as alterações.
func (a *Analyzer) Analyze(all []*git.FileChange) *Result {
	var files []*git.FileChange
	for _, f := range all {
		if !matchAny(a.ignore, f.Path) {
			files = append(files, f)
		}
	}

	res := &Result{}
	var findings []Finding
	for _, f := range files {
		res.Summary.Files++
		res.Summary.Added += len(f.Added)
		res.Summary.Removed += f.Removed

		findings = append(findings, a.fileFindings(f)...)
		if f.Binary || f.Status == git.StatusDeleted {
			continue
		}
		findings = append(findings, a.lineFindings(f)...)
	}
	findings = append(findings, a.scopeFindings(files)...)

	kept := make([]Finding, 0, len(findings))
	for _, fd := range findings {
		if !a.disabled[fd.Rule] {
			kept = append(kept, fd)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		x, y := kept[i], kept[j]
		if (x.File == "") != (y.File == "") {
			return y.File == ""
		}
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Severity > y.Severity
	})

	for _, fd := range kept {
		switch fd.Severity {
		case Error:
			res.Summary.Errors++
		case Warning:
			res.Summary.Warnings++
		default:
			res.Summary.Infos++
		}
	}
	res.Findings = kept
	return res
}

func (a *Analyzer) fileFindings(f *git.FileChange) []Finding {
	if f.Status == git.StatusDeleted || matchAny(a.allow, f.Path) {
		return nil
	}
	var out []Finding
	for _, fr := range a.fileRules {
		if fr.re.MatchString(f.Path) && !matchAny(fr.except, f.Path) {
			out = append(out, Finding{
				Rule: fr.ID, Category: "file", Severity: fr.Severity, File: f.Path, Message: fr.Message,
			})
			break // um achado de "arquivo indesejado" por arquivo basta
		}
	}
	if limit := int64(a.cfg.Limits.MaxFileSizeKB) * 1024; limit > 0 && f.Size > limit {
		out = append(out, Finding{
			Rule: "large-file", Category: "file", Severity: Warning, File: f.Path,
			Message: fmt.Sprintf("Arquivo de %s (limite: %s)", humanSize(f.Size), humanSize(limit)),
		})
	}
	return out
}

func (a *Analyzer) lineFindings(f *git.FileChange) []Finding {
	var out []Finding
	skipCode := classify.IsGenerated(f.Path) || classify.Of(f.Path) == classify.Docs
	for _, ln := range f.Added {
		text := ln.Text
		if len(text) > maxLineLen {
			continue
		}
		for i := range a.lineRules {
			r := &a.lineRules[i]
			if r.CodeOnly && skipCode {
				continue
			}
			if !r.applies(f.Path) || !r.Re.MatchString(text) {
				continue
			}
			if ignoredByComment(text, r.ID) {
				continue
			}
			out = append(out, Finding{
				Rule: r.ID, Category: r.Category, Severity: r.Severity,
				File: f.Path, Line: ln.No, Message: r.Message,
				Snippet: makeSnippet(r, text),
			})
		}
	}
	return out
}

// ignoredByComment reconhece "cleancommit:ignore" e "cleancommit:ignore=regra1,regra2".
func ignoredByComment(text, id string) bool {
	i := strings.Index(text, ignoreTag)
	if i < 0 {
		return false
	}
	rest := text[i+len(ignoreTag):]
	if !strings.HasPrefix(rest, "=") {
		return true
	}
	rest = rest[1:]
	if j := strings.IndexAny(rest, " \t*/"); j >= 0 {
		rest = rest[:j]
	}
	for _, item := range strings.Split(rest, ",") {
		if strings.TrimSpace(item) == id {
			return true
		}
	}
	return false
}

func makeSnippet(r *lineRule, text string) string {
	if r.Category == "secret" {
		if loc := r.Re.FindStringIndex(text); loc != nil {
			m := []rune(text[loc[0]:loc[1]])
			keep := 4
			if len(m) < keep {
				keep = len(m)
			}
			return strings.TrimSpace(text[:loc[0]]+string(m[:keep])) + "••••"
		}
	}
	s := []rune(strings.TrimSpace(text))
	if len(s) > 100 {
		return string(s[:99]) + "…"
	}
	return string(s)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
