// Package hooks instala e remove os Git hooks do CleanCommit.
package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	markerStart = "# >>> cleancommit >>>"
	markerEnd   = "# <<< cleancommit <<<"
)

// Nomes de hooks suportados.
const (
	PreCommit        = "pre-commit"
	PrepareCommitMsg = "prepare-commit-msg"
)

func block(name string) string {
	var body string
	switch name {
	case PrepareCommitMsg:
		body = `  cleancommit hook-msg "$1" "$2" || true`
	default:
		body = `  cleancommit check --hook || exit $?`
	}
	return markerStart + "\n" +
		`if [ "${CLEANCOMMIT_SKIP:-0}" != "1" ] && command -v cleancommit >/dev/null 2>&1; then` + "\n" +
		body + "\n" +
		"fi\n" +
		markerEnd + "\n"
}

// Install cria ou atualiza o bloco do CleanCommit em um hook.
// Retorna "criado", "atualizado" ou "anexado".
func Install(path, name string, force bool) (string, error) {
	blk := block(name)
	existing, err := os.ReadFile(path)
	var content, action string

	switch {
	case err != nil && os.IsNotExist(err):
		content, action = "#!/bin/sh\n"+blk, "criado"
	case err != nil:
		return "", err
	case strings.Contains(string(existing), markerStart):
		content, action = replaceBlock(string(existing), blk), "atualizado"
	default:
		if !force {
			return "", fmt.Errorf("já existe um hook %s que não é do CleanCommit (use --force para anexar o bloco ao final dele)", name)
		}
		text := string(existing)
		if strings.HasPrefix(text, "#!") {
			first := text
			if i := strings.Index(text, "\n"); i >= 0 {
				first = text[:i]
			}
			if !strings.Contains(first, "sh") {
				return "", errors.New("o hook existente não é um script de shell; adicione manualmente a chamada `cleancommit check --hook`")
			}
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		content, action = text+blk, "anexado"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", err
	}
	return action, nil
}

// Uninstall remove o bloco do CleanCommit. Devolve false se não havia bloco.
func Uninstall(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	text := string(data)
	if !strings.Contains(text, markerStart) {
		return false, nil
	}
	rest := removeBlock(text)
	if t := strings.TrimSpace(rest); t == "" || t == "#!/bin/sh" {
		return true, os.Remove(path)
	}
	return true, os.WriteFile(path, []byte(rest), 0o755)
}

// Installed informa se o hook contém o bloco do CleanCommit.
func Installed(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), markerStart)
}

func blockBounds(s string) (start, end int, ok bool) {
	start = strings.Index(s, markerStart)
	if start < 0 {
		return 0, 0, false
	}
	rel := strings.Index(s[start:], markerEnd)
	if rel < 0 {
		return 0, 0, false
	}
	end = start + rel + len(markerEnd)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return start, end, true
}

func removeBlock(s string) string {
	start, end, ok := blockBounds(s)
	if !ok {
		return s
	}
	return s[:start] + s[end:]
}

func replaceBlock(s, blk string) string {
	start, end, ok := blockBounds(s)
	if !ok {
		return s + blk
	}
	return s[:start] + blk + s[end:]
}

// PrefillMessage grava a mensagem sugerida no arquivo de mensagem do commit
// (usado pelo hook prepare-commit-msg), sem sobrescrever mensagens existentes.
func PrefillMessage(file, source, message string) error {
	switch source {
	case "message", "merge", "squash", "commit":
		return nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			return nil // já existe uma mensagem
		}
	}
	out := message + "\n\n# Mensagem sugerida pelo CleanCommit. Edite à vontade.\n" + string(data)
	return os.WriteFile(file, []byte(out), 0o644)
}
