package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSampleIsValid(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte(Sample()), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("o exemplo gerado por init deve ser válido: %v", err)
	}
	if len(cfg.Rules) != 1 || cfg.Rules[0].ID != "no-fixme-in-main" {
		t.Errorf("regras inesperadas: %+v", cfg.Rules)
	}
}

func TestPartialKeepsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte(`{"limits": {"max_files": 5}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.MaxFiles != 5 || cfg.Limits.MaxLines != 800 || cfg.FailOn != "error" {
		t.Errorf("padrões não preservados: %+v", cfg)
	}
}

func TestInvalid(t *testing.T) {
	cases := []string{
		`{"fail_on": "talvez"}`,
		`{"campo_inexistente": true}`,
		`{"rules": [{"id": "x", "pattern": "("}]}`,
		`{"rules": [{"pattern": "x"}]}`,
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), FileName)
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Errorf("deveria rejeitar: %s", c)
		}
	}
}
