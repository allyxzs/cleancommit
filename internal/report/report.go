// Package report desenha a saída do CleanCommit no terminal.
package report

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"cleancommit/internal/analyzer"
	"cleancommit/internal/suggest"
)

// Style liga ou desliga as cores ANSI.
type Style struct {
	On bool
}

// ColorEnabled decide se vale usar cores no arquivo de saída informado.
func ColorEnabled(noColor bool, f *os.File) bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR_FORCE") == "1" || os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (s Style) wrap(code, text string) string {
	if !s.On {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(t string) string    { return s.wrap("1", t) }
func (s Style) Dim(t string) string     { return s.wrap("2", t) }
func (s Style) Red(t string) string     { return s.wrap("31", t) }
func (s Style) Green(t string) string   { return s.wrap("32", t) }
func (s Style) Yellow(t string) string  { return s.wrap("33", t) }
func (s Style) Blue(t string) string    { return s.wrap("34", t) }
func (s Style) Cyan(t string) string    { return s.wrap("36", t) }
func (s Style) BoldRed(t string) string { return s.wrap("1;31", t) }

func (s Style) icon(sev analyzer.Severity) string {
	switch sev {
	case analyzer.Error:
		return s.Red("✖")
	case analyzer.Warning:
		return s.Yellow("▲")
	}
	return s.Blue("●")
}

func (s Style) paint(sev analyzer.Severity, t string) string {
	switch sev {
	case analyzer.Error:
		return s.Red(t)
	case analyzer.Warning:
		return s.Yellow(t)
	}
	return s.Blue(t)
}

func padRight(t string, n int) string {
	if c := utf8.RuneCountInString(t); c < n {
		return t + strings.Repeat(" ", n-c)
	}
	return t
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// Options controla o texto do relatório.
type Options struct {
	Mode    string // ex.: "em stage"
	FailOn  string
	Blocked bool
	Hook    bool
}

// Render escreve o relatório de uma análise.
func Render(w io.Writer, res *analyzer.Result, st Style, opt Options) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, " %s %s %s\n", st.Bold(st.Cyan("CleanCommit")), st.Dim("›"), st.Dim("revisando alterações "+opt.Mode))
	fmt.Fprintf(w, " %s  %s %s\n\n", plural(res.Summary.Files, "arquivo", "arquivos"),
		st.Green(fmt.Sprintf("+%d", res.Summary.Added)), st.Red(fmt.Sprintf("−%d", res.Summary.Removed)))

	if len(res.Findings) == 0 {
		fmt.Fprintf(w, " %s Nada suspeito encontrado — pode commitar.\n\n", st.Green("✔"))
		return
	}

	ruleW := 0
	for _, f := range res.Findings {
		if n := utf8.RuneCountInString(f.Rule); n > ruleW {
			ruleW = n
		}
	}
	if ruleW > 24 {
		ruleW = 24
	}

	current := "\x00"
	for i, f := range res.Findings {
		if f.File != current {
			current = f.File
			title := f.File
			if title == "" {
				title = "Escopo do commit"
			}
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, " %s\n", st.Bold(title))
		}
		loc := "     "
		if f.Line > 0 {
			loc = padRight("L"+strconv.Itoa(f.Line), 5)
		}
		fmt.Fprintf(w, "   %s %s %s %s\n", st.icon(f.Severity), st.Dim(loc), st.paint(f.Severity, padRight(f.Rule, ruleW)), f.Message)
		if f.Snippet != "" {
			fmt.Fprintf(w, "         %s %s\n", st.Dim("│"), st.Dim(f.Snippet))
		}
	}

	fmt.Fprintf(w, "\n %s\n", st.Dim(strings.Repeat("─", 60)))
	var parts []string
	if n := res.Summary.Errors; n > 0 {
		parts = append(parts, st.Red("✖ "+plural(n, "erro", "erros")))
	}
	if n := res.Summary.Warnings; n > 0 {
		parts = append(parts, st.Yellow("▲ "+plural(n, "aviso", "avisos")))
	}
	if n := res.Summary.Infos; n > 0 {
		parts = append(parts, st.Blue("● "+plural(n, "info", "infos")))
	}
	fmt.Fprintf(w, " %s\n", strings.Join(parts, "   "))

	if opt.Blocked {
		hint := " — corrija os itens acima antes de commitar"
		if opt.Hook {
			hint = " — corrija os itens acima ou use git commit --no-verify"
		}
		fmt.Fprintf(w, " %s%s\n\n", st.BoldRed("Commit bloqueado"), st.Dim(hint))
		return
	}
	fmt.Fprintf(w, " %s%s\n\n", st.Green("✔ Pode commitar"), st.Dim(" — nada atinge o nível de bloqueio ("+opt.FailOn+")"))
}

// RenderSuggestion desenha a mensagem de commit sugerida.
func RenderSuggestion(w io.Writer, s *suggest.Suggestion, st Style) {
	lines := strings.Split(s.Message(), "\n")
	width := 44
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > width {
			width = n
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, " %s %s %s\n", st.Bold(st.Cyan("CleanCommit")), st.Dim("›"), st.Dim("mensagem sugerida"))
	fmt.Fprintf(w, " %s\n", st.Dim("╭"+strings.Repeat("─", width+2)+"╮"))
	for i, l := range lines {
		text := padRight(l, width)
		if i == 0 {
			text = st.Bold(st.Green(text))
		}
		fmt.Fprintf(w, " %s %s %s\n", st.Dim("│"), text, st.Dim("│"))
	}
	fmt.Fprintf(w, " %s\n", st.Dim("╰"+strings.Repeat("─", width+2)+"╯"))

	if len(s.Alternatives) > 0 {
		var alts []string
		for _, t := range s.Alternatives {
			alts = append(alts, s.HeaderFor(t))
		}
		fmt.Fprintf(w, " %s\n", st.Dim("Outros tipos possíveis:"))
		for _, a := range alts {
			fmt.Fprintf(w, "   %s\n", st.Dim(a))
		}
	}
	fmt.Fprintf(w, "\n %s\n\n", st.Dim("Para usar:  cleancommit suggest --commit"))
}

var categoryLabels = []struct{ key, label string }{
	{"secret", "Segredos"},
	{"conflict", "Conflitos"},
	{"debug", "Depuração"},
	{"marker", "Marcadores"},
	{"file", "Arquivos indesejados"},
	{"scope", "Escopo e tamanho"},
}

// RenderRules lista as regras embutidas agrupadas por categoria.
func RenderRules(w io.Writer, rules []analyzer.RuleInfo, st Style) {
	width := 0
	for _, r := range rules {
		if n := utf8.RuneCountInString(r.ID); n > width {
			width = n
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, " %s %s %s\n", st.Bold(st.Cyan("CleanCommit")), st.Dim("›"), st.Dim("regras embutidas"))
	for _, c := range categoryLabels {
		printed := false
		for _, r := range rules {
			if r.Category != c.key {
				continue
			}
			if !printed {
				fmt.Fprintf(w, "\n %s\n", st.Bold(c.label))
				printed = true
			}
			fmt.Fprintf(w, "   %s %s %s\n", st.icon(r.Severity), st.paint(r.Severity, padRight(r.ID, width)), r.Description)
		}
	}
	fmt.Fprintf(w, "\n %s\n\n", st.Dim("Desative qualquer regra em .cleancommit.json (\"disable\") ou na linha, com // cleancommit:ignore"))
}
