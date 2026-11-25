package suggest

import (
	"strings"
	"testing"

	"cleancommit/internal/git"
)

func lines(n int) []git.AddedLine {
	out := make([]git.AddedLine, n)
	for i := range out {
		out[i] = git.AddedLine{No: i + 1, Text: "x"}
	}
	return out
}

func TestNewCodeIsFeat(t *testing.T) {
	files := []*git.FileChange{
		{Path: "internal/auth/token.go", Status: git.StatusAdded, Added: lines(30)},
	}
	s := Build(files, "main", "pt")
	if got := s.Header(); got != "feat(auth): adiciona token" {
		t.Errorf("cabeçalho inesperado: %q", got)
	}
	if s.Body != "" || s.Footer != "" {
		t.Errorf("corpo/rodapé deveriam estar vazios: %q / %q", s.Body, s.Footer)
	}
}

func TestDocsOnly(t *testing.T) {
	files := []*git.FileChange{{Path: "README.md", Status: git.StatusModified, Added: lines(3)}}
	if got := Build(files, "", "pt").Header(); got != "docs: atualiza readme" {
		t.Errorf("cabeçalho inesperado: %q", got)
	}
}

func TestBranchHintAndTicket(t *testing.T) {
	files := []*git.FileChange{
		{Path: "src/api/user.ts", Status: git.StatusModified, Added: lines(2), Removed: 1},
	}
	s := Build(files, "fix/123-login", "en")
	if s.Type != "fix" {
		t.Errorf("tipo deveria vir da branch: %q", s.Type)
	}
	if s.Footer != "Refs: #123" {
		t.Errorf("rodapé inesperado: %q", s.Footer)
	}
	s = Build(files, "feature/PROJ-42-pagamento", "en")
	if s.Type != "feat" || s.Footer != "Refs: PROJ-42" {
		t.Errorf("sugestão inesperada: %q / %q", s.Type, s.Footer)
	}
}

func TestMultipleFilesBody(t *testing.T) {
	files := []*git.FileChange{
		{Path: "internal/auth/a.go", Status: git.StatusModified, Added: lines(5)},
		{Path: "internal/auth/b.go", Status: git.StatusModified, Added: lines(9)},
	}
	s := Build(files, "", "pt")
	if s.Scope != "auth" {
		t.Errorf("escopo inesperado: %q", s.Scope)
	}
	if !strings.HasPrefix(s.Body, "- internal/auth/b.go (+9 -0)") {
		t.Errorf("corpo deveria listar o maior arquivo primeiro: %q", s.Body)
	}
	if !strings.Contains(s.Subject, "a e b") && !strings.Contains(s.Subject, "b e a") {
		t.Errorf("assunto deveria citar os dois arquivos: %q", s.Subject)
	}
}

func TestHeaderLengthLimit(t *testing.T) {
	var files []*git.FileChange
	for _, n := range []string{"muito_longo_nome_de_arquivo_um", "muito_longo_nome_de_arquivo_dois", "muito_longo_nome_de_arquivo_tres"} {
		files = append(files, &git.FileChange{Path: "pkg/x/" + n + ".go", Status: git.StatusModified, Added: lines(2)})
	}
	if n := len([]rune(Build(files, "", "pt").Header())); n > maxHeader {
		t.Errorf("cabeçalho com %d caracteres (limite %d)", n, maxHeader)
	}
}

func TestLockfileOnlyIsChoreDeps(t *testing.T) {
	files := []*git.FileChange{{Path: "package-lock.json", Status: git.StatusModified, Added: lines(40)}}
	if got := Build(files, "", "pt").Header(); got != "chore(deps): atualiza dependências" {
		t.Errorf("cabeçalho inesperado: %q", got)
	}
}
