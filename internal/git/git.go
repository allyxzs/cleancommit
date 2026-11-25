// Package git encapsula as chamadas ao binário git e o parsing dos diffs.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Repo representa o repositório Git que contém o diretório atual.
type Repo struct {
	Root string
}

// Mode define qual conjunto de alterações será analisado.
type Mode int

const (
	// Staged analisa o que está no stage (o que entraria no próximo commit).
	Staged Mode = iota
	// Working analisa o diretório de trabalho contra o HEAD.
	Working
)

// Open localiza o repositório a partir do diretório atual.
func Open() (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errors.New("git não encontrado no PATH")
	}
	out, err := run("", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errors.New("este diretório não está dentro de um repositório Git")
	}
	return &Repo{Root: strings.TrimSpace(out)}, nil
}

func run(dir string, args ...string) (string, error) {
	full := append([]string{"-c", "core.quotepath=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// Run executa um comando git na raiz do repositório.
func (r *Repo) Run(args ...string) (string, error) {
	return run(r.Root, args...)
}

// Branch devolve o nome da branch atual ("" em HEAD destacado).
func (r *Repo) Branch() string {
	out, err := r.Run("symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// HasHead informa se já existe ao menos um commit.
func (r *Repo) HasHead() bool {
	_, err := r.Run("rev-parse", "--verify", "-q", "HEAD")
	return err == nil
}

// HookPath devolve o caminho absoluto de um hook, respeitando core.hooksPath.
func (r *Repo) HookPath(name string) (string, error) {
	out, err := r.Run("rev-parse", "--git-path", "hooks/"+name)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Root, p)
	}
	return p, nil
}

// Commit cria um commit usando cada mensagem como um parágrafo (-m).
func (r *Repo) Commit(messages ...string) error {
	args := []string{"commit"}
	for _, m := range messages {
		if m != "" {
			args = append(args, "-m", m)
		}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func diffBase() []string {
	return []string{
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
	}
}

func (r *Repo) modeArgs(mode Mode) []string {
	switch mode {
	case Staged:
		return []string{"--cached"}
	case Working:
		if r.HasHead() {
			return []string{"HEAD"}
		}
	}
	return nil
}

// Changes lê as alterações do modo escolhido e devolve uma lista de arquivos.
func (r *Repo) Changes(mode Mode) ([]*FileChange, error) {
	args := append(diffBase(), "-M", "--unified=0")
	args = append(args, r.modeArgs(mode)...)
	out, err := r.Run(args...)
	if err != nil {
		return nil, err
	}
	files := ParseDiff(out)
	if len(files) == 0 {
		return files, nil
	}

	all, okAll := r.numstat(mode, false)
	ws, okWs := r.numstat(mode, true)
	for _, f := range files {
		if f.Status == StatusDeleted {
			continue
		}
		f.Size = r.blobSize(f.Path, mode)
		if okAll && okWs && f.Status == StatusModified && !f.Binary {
			a, found := all[f.Path]
			if !found || a.added+a.removed == 0 {
				continue
			}
			w, found := ws[f.Path]
			if !found || w.added+w.removed == 0 {
				f.WhitespaceOnly = true
			}
		}
	}
	return files, nil
}

type numstat struct {
	added, removed int
}

func (r *Repo) numstat(mode Mode, ignoreWhitespace bool) (map[string]numstat, bool) {
	args := append(diffBase(), "--no-renames", "--numstat")
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = append(args, r.modeArgs(mode)...)
	out, err := r.Run(args...)
	if err != nil {
		return nil, false
	}
	res := map[string]numstat{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		a, errA := strconv.Atoi(parts[0])
		d, errD := strconv.Atoi(parts[1])
		if errA != nil || errD != nil {
			continue // binário
		}
		res[parts[2]] = numstat{added: a, removed: d}
	}
	return res, true
}

func (r *Repo) blobSize(path string, mode Mode) int64 {
	if mode == Working {
		st, err := os.Stat(filepath.Join(r.Root, filepath.FromSlash(path)))
		if err != nil {
			return 0
		}
		return st.Size()
	}
	out, err := r.Run("cat-file", "-s", ":"+path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
