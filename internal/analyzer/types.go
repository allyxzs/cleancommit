// Package analyzer aplica as regras do CleanCommit sobre as alterações do Git.
package analyzer

import "encoding/json"

// Severity indica a gravidade de um achado.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "info"
}

// MarshalJSON serializa a severidade como texto.
func (s Severity) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// ParseSeverity converte "info", "warning" ou "error".
func ParseSeverity(s string) (Severity, bool) {
	switch s {
	case "info":
		return Info, true
	case "warning":
		return Warning, true
	case "error":
		return Error, true
	}
	return Info, false
}

// Finding é um problema encontrado.
type Finding struct {
	Rule     string   `json:"rule"`
	Category string   `json:"category"`
	Severity Severity `json:"severity"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Snippet  string   `json:"snippet,omitempty"`
	Message  string   `json:"message"`
}

// Summary reúne os totais da análise.
type Summary struct {
	Files    int `json:"files"`
	Added    int `json:"added"`
	Removed  int `json:"removed"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// Result é o resultado completo de uma análise.
type Result struct {
	Findings []Finding `json:"findings"`
	Summary  Summary   `json:"summary"`
}

// Blocked informa se algum achado atinge o nível de bloqueio (fail_on).
func (r *Result) Blocked(failOn string) bool {
	var threshold Severity
	switch failOn {
	case "never":
		return false
	case "info":
		threshold = Info
	case "warning":
		threshold = Warning
	default:
		threshold = Error
	}
	for _, f := range r.Findings {
		if f.Severity >= threshold {
			return true
		}
	}
	return false
}

// RuleInfo descreve uma regra embutida (usado por `cleancommit rules`).
type RuleInfo struct {
	ID          string
	Category    string
	Severity    Severity
	Description string
}
