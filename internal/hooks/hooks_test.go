package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUninstall(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hooks", "pre-commit")

	if act, err := Install(p, PreCommit, false); err != nil || act != "criado" {
		t.Fatalf("install: %q, %v", act, err)
	}
	if !Installed(p) {
		t.Fatal("hook deveria estar instalado")
	}
	if act, err := Install(p, PreCommit, false); err != nil || act != "atualizado" {
		t.Fatalf("reinstall: %q, %v", act, err)
	}
	data, _ := os.ReadFile(p)
	if n := strings.Count(string(data), markerStart); n != 1 {
		t.Fatalf("o bloco deveria aparecer uma vez, apareceu %d", n)
	}
	if !strings.Contains(string(data), "cleancommit check --hook") {
		t.Fatal("hook sem a chamada do CleanCommit")
	}

	if removed, err := Uninstall(p); err != nil || !removed {
		t.Fatalf("uninstall: %v, %v", removed, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("o arquivo deveria ter sido removido")
	}
}

func TestForeignHookIsPreserved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pre-commit")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho oi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(p, PreCommit, false); err == nil {
		t.Fatal("sem --force não deveria mexer num hook alheio")
	}
	if act, err := Install(p, PreCommit, true); err != nil || act != "anexado" {
		t.Fatalf("force: %q, %v", act, err)
	}
	if _, err := Uninstall(p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "#!/bin/sh\necho oi" {
		t.Fatalf("o hook original deveria ter sido preservado: %q", data)
	}
}

func TestPrefillMessage(t *testing.T) {
	f := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(f, []byte("\n# Please enter the commit message\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PrefillMessage(f, "", "feat: algo"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	if !strings.HasPrefix(string(data), "feat: algo\n") {
		t.Fatalf("mensagem não foi preenchida: %q", data)
	}

	// Se o usuário já passou -m, nada muda.
	if err := os.WriteFile(f, []byte("minha mensagem\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PrefillMessage(f, "message", "feat: algo"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(f)
	if string(data) != "minha mensagem\n" {
		t.Fatalf("mensagem do usuário foi alterada: %q", data)
	}
}
