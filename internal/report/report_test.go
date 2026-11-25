package report

import (
	"bytes"
	"strings"
	"testing"

	"cleancommit/internal/analyzer"
)

func TestRenderPlain(t *testing.T) {
	res := &analyzer.Result{
		Findings: []analyzer.Finding{
			{Rule: "js-debugger", Category: "debug", Severity: analyzer.Error, File: "a.js", Line: 3, Message: "debugger esquecido", Snippet: "debugger;"},
			{Rule: "big-commit-files", Category: "scope", Severity: analyzer.Warning, Message: "commit grande"},
		},
		Summary: analyzer.Summary{Files: 2, Added: 10, Removed: 1, Errors: 1, Warnings: 1},
	}
	var buf bytes.Buffer
	Render(&buf, res, Style{On: false}, Options{Mode: "em stage", FailOn: "error", Blocked: true})
	out := buf.String()
	for _, want := range []string{"a.js", "L3", "js-debugger", "debugger;", "Escopo do commit", "Commit bloqueado", "1 erro", "1 aviso"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída sem %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("sem cores não pode haver códigos ANSI")
	}
}

func TestRenderClean(t *testing.T) {
	var buf bytes.Buffer
	Render(&buf, &analyzer.Result{}, Style{}, Options{Mode: "em stage"})
	if !strings.Contains(buf.String(), "Nada suspeito") {
		t.Errorf("saída inesperada: %s", buf.String())
	}
}
