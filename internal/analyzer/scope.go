package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"cleancommit/internal/classify"
	"cleancommit/internal/git"
)

func scopeFinding(rule string, msg string) Finding {
	return Finding{Rule: rule, Category: "scope", Severity: Warning, Message: msg}
}

var groupLabels = map[classify.Kind]string{
	classify.Code:   "código",
	classify.Test:   "código",
	classify.Docs:   "docs",
	classify.CI:     "CI",
	classify.Deps:   "dependências",
	classify.Build:  "build",
	classify.Config: "configuração",
	classify.Asset:  "assets",
}

// scopeFindings detecta commits grandes demais ou misturados.
func (a *Analyzer) scopeFindings(files []*git.FileChange) []Finding {
	lim := a.cfg.Limits
	var out []Finding

	changed := map[string]bool{}
	for _, f := range files {
		changed[f.Path] = true
	}

	counted, total, deleted := 0, 0, 0
	groups := map[string]bool{}
	modules := map[string]bool{}

	for _, f := range files {
		kind := classify.Of(f.Path)
		if f.Status == git.StatusDeleted {
			deleted++
		}

		if kind == classify.Generated {
			if f.Status != git.StatusDeleted {
				if m, ok := classify.Manifest(f.Path); ok && !changed[m] {
					fd := scopeFinding("lockfile-only", fmt.Sprintf("Lockfile mudou sem %s — atualização acidental de dependências?", m))
					fd.File = f.Path
					out = append(out, fd)
				}
			}
			continue
		}

		counted++
		churn := f.Churn()
		total += churn

		if lim.MaxFileLines > 0 && churn > lim.MaxFileLines && f.Status != git.StatusDeleted && !f.Binary {
			fd := scopeFinding("huge-file-change", fmt.Sprintf("%d linhas alteradas neste arquivo (limite: %d) — tudo isso é intencional?", churn, lim.MaxFileLines))
			fd.File = f.Path
			out = append(out, fd)
		}
		if f.WhitespaceOnly {
			fd := scopeFinding("whitespace-only", "Só espaços/indentação mudaram — ruído no diff?")
			fd.File = f.Path
			out = append(out, fd)
		}

		if label, ok := groupLabels[kind]; ok {
			groups[label] = true
		}
		if kind == classify.Code || kind == classify.Test {
			if m := classify.Module(f.Path); m != "" {
				modules[m] = true
			}
		}
	}

	if lim.MaxFiles > 0 && counted > lim.MaxFiles {
		out = append(out, scopeFinding("big-commit-files",
			fmt.Sprintf("%d arquivos alterados (limite: %d) — considere dividir em commits menores", counted, lim.MaxFiles)))
	}
	if lim.MaxLines > 0 && total > lim.MaxLines {
		out = append(out, scopeFinding("big-commit-lines",
			fmt.Sprintf("%d linhas alteradas (limite: %d) — considere dividir em commits menores", total, lim.MaxLines)))
	}
	if len(groups) >= 4 {
		labels := make([]string, 0, len(groups))
		for l := range groups {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		out = append(out, scopeFinding("mixed-concerns",
			fmt.Sprintf("Mudanças de naturezas diferentes no mesmo commit (%s) — separar facilita revisão e revert", strings.Join(labels, ", "))))
	}
	if len(modules) >= 6 {
		out = append(out, scopeFinding("scattered-changes",
			fmt.Sprintf("Alterações espalhadas por %d módulos — o commit está focado em uma única coisa?", len(modules))))
	}
	if deleted >= 10 {
		out = append(out, scopeFinding("mass-deletion",
			fmt.Sprintf("%d arquivos removidos de uma vez — confirme que foi intencional", deleted)))
	}
	return out
}
