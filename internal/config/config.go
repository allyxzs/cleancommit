// Package config carrega e valida o arquivo .cleancommit.json.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileName é o nome do arquivo de configuração na raiz do repositório.
const FileName = ".cleancommit.json"

// Limits define os limites da análise de alterações excessivas (0 desativa).
type Limits struct {
	MaxFiles      int `json:"max_files"`
	MaxLines      int `json:"max_lines"`
	MaxFileLines  int `json:"max_file_lines"`
	MaxFileSizeKB int `json:"max_file_size_kb"`
}

// CustomRule é uma regra por expressão regular aplicada às linhas adicionadas.
type CustomRule struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Pattern     string   `json:"pattern"`
	Severity    string   `json:"severity"`
	Paths       []string `json:"paths"`
	Message     string   `json:"message"`
}

// ForbiddenFile marca arquivos que não devem ser commitados.
type ForbiddenFile struct {
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Config é a configuração completa.
type Config struct {
	Schema         string          `json:"$schema,omitempty"`
	FailOn         string          `json:"fail_on"`
	Language       string          `json:"language"`
	Limits         Limits          `json:"limits"`
	IgnorePaths    []string        `json:"ignore_paths"`
	AllowFiles     []string        `json:"allow_files"`
	Disable        []string        `json:"disable"`
	Rules          []CustomRule    `json:"rules"`
	ForbiddenFiles []ForbiddenFile `json:"forbidden_files"`
}

// Default devolve a configuração padrão.
func Default() *Config {
	return &Config{
		FailOn:   "error",
		Language: "pt",
		Limits: Limits{
			MaxFiles:      30,
			MaxLines:      800,
			MaxFileLines:  400,
			MaxFileSizeKB: 1024,
		},
	}
}

// Discover procura o arquivo de configuração na raiz do repositório.
func Discover(root string) string {
	p := filepath.Join(root, FileName)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// Load lê e valida um arquivo de configuração. Campos ausentes mantêm o padrão.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// ValidFailOn informa se o nível é aceito em fail_on / --fail-on.
func ValidFailOn(s string) bool {
	switch s {
	case "error", "warning", "info", "never":
		return true
	}
	return false
}

func validSeverity(s string) bool {
	switch s {
	case "error", "warning", "info":
		return true
	}
	return false
}

// Validate confere os valores e preenche padrões.
func (c *Config) Validate() error {
	c.FailOn = strings.ToLower(strings.TrimSpace(c.FailOn))
	if !ValidFailOn(c.FailOn) {
		return fmt.Errorf("fail_on inválido %q (use error, warning, info ou never)", c.FailOn)
	}
	c.Language = strings.ToLower(strings.TrimSpace(c.Language))
	if c.Language != "pt" && c.Language != "en" {
		return fmt.Errorf("language inválido %q (use pt ou en)", c.Language)
	}
	l := c.Limits
	if l.MaxFiles < 0 || l.MaxLines < 0 || l.MaxFileLines < 0 || l.MaxFileSizeKB < 0 {
		return errors.New("limits não aceita valores negativos (use 0 para desativar)")
	}

	seen := map[string]bool{}
	for i := range c.Rules {
		r := &c.Rules[i]
		r.ID = strings.TrimSpace(r.ID)
		if r.ID == "" {
			return fmt.Errorf("rules[%d]: o campo id é obrigatório", i)
		}
		if seen[r.ID] {
			return fmt.Errorf("rules[%d]: id %q repetido", i, r.ID)
		}
		seen[r.ID] = true
		if r.Pattern == "" {
			return fmt.Errorf("rules[%d] (%s): o campo pattern é obrigatório", i, r.ID)
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("rules[%d] (%s): pattern inválido: %v", i, r.ID, err)
		}
		r.Severity = strings.ToLower(strings.TrimSpace(r.Severity))
		if r.Severity == "" {
			r.Severity = "warning"
		}
		if !validSeverity(r.Severity) {
			return fmt.Errorf("rules[%d] (%s): severity inválida %q", i, r.ID, r.Severity)
		}
	}
	for i := range c.ForbiddenFiles {
		f := &c.ForbiddenFiles[i]
		if strings.TrimSpace(f.Pattern) == "" {
			return fmt.Errorf("forbidden_files[%d]: o campo pattern é obrigatório", i)
		}
		f.Severity = strings.ToLower(strings.TrimSpace(f.Severity))
		if f.Severity == "" {
			f.Severity = "error"
		}
		if !validSeverity(f.Severity) {
			return fmt.Errorf("forbidden_files[%d]: severity inválida %q", i, f.Severity)
		}
	}
	return nil
}

// Sample é o conteúdo gerado por `cleancommit init`.
func Sample() string {
	return `{
  "fail_on": "error",
  "language": "pt",
  "limits": {
    "max_files": 30,
    "max_lines": 800,
    "max_file_lines": 400,
    "max_file_size_kb": 1024
  },
  "ignore_paths": ["vendor/", "docs/legacy/**"],
  "allow_files": [],
  "disable": [],
  "forbidden_files": [
    {
      "pattern": "*.sqlite",
      "severity": "error",
      "message": "Bancos locais não devem ser versionados"
    }
  ],
  "rules": [
    {
      "id": "no-fixme-in-main",
      "description": "Bloqueia FIXME em código novo",
      "pattern": "FIXME",
      "severity": "error",
      "paths": ["src/**", "internal/**"],
      "message": "Resolva o FIXME antes de commitar"
    }
  ]
}
`
}
