package git

import (
	"strconv"
	"strings"
)

// Status descreve o que aconteceu com um arquivo.
type Status string

const (
	StatusAdded    Status = "A"
	StatusModified Status = "M"
	StatusDeleted  Status = "D"
	StatusRenamed  Status = "R"
)

// AddedLine é uma linha adicionada, com o número na versão nova do arquivo.
type AddedLine struct {
	No   int
	Text string
}

// FileChange resume as alterações de um arquivo.
type FileChange struct {
	Path           string
	OldPath        string
	Status         Status
	Binary         bool
	Added          []AddedLine
	Removed        int
	Size           int64
	WhitespaceOnly bool
}

// Churn é o total de linhas adicionadas e removidas.
func (f *FileChange) Churn() int {
	return len(f.Added) + f.Removed
}

// ParseDiff interpreta a saída de `git diff --unified=0`.
func ParseDiff(out string) []*FileChange {
	var files []*FileChange
	var cur *FileChange
	newLine := 0
	inHunk := false

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			cur = &FileChange{Status: StatusModified}
			header := strings.TrimPrefix(line, "diff --git ")
			if i := strings.LastIndex(header, " b/"); i >= 0 {
				cur.Path = header[i+3:]
				cur.OldPath = strings.TrimPrefix(header[:i], "a/")
			}
			files = append(files, cur)
			inHunk = false
		case cur == nil:
			continue
		case !inHunk && strings.HasPrefix(line, "new file mode"):
			cur.Status = StatusAdded
		case !inHunk && strings.HasPrefix(line, "deleted file mode"):
			cur.Status = StatusDeleted
		case !inHunk && strings.HasPrefix(line, "rename from "):
			cur.Status = StatusRenamed
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case !inHunk && strings.HasPrefix(line, "rename to "):
			cur.Status = StatusRenamed
			cur.Path = strings.TrimPrefix(line, "rename to ")
		case !inHunk && (strings.HasPrefix(line, "Binary files") || strings.HasPrefix(line, "GIT binary patch")):
			cur.Binary = true
		case !inHunk && strings.HasPrefix(line, "+++ "):
			p := strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t")
			if strings.HasPrefix(p, "b/") {
				cur.Path = p[2:]
			}
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			newLine = parseHunkStart(line)
		case inHunk && strings.HasPrefix(line, "+"):
			cur.Added = append(cur.Added, AddedLine{No: newLine, Text: strings.TrimSuffix(line[1:], "\r")})
			newLine++
		case inHunk && strings.HasPrefix(line, "-"):
			cur.Removed++
		}
	}
	return files
}

// parseHunkStart extrai o número da primeira linha nova de "@@ -a,b +c,d @@".
func parseHunkStart(h string) int {
	i := strings.Index(h, " +")
	if i < 0 {
		return 0
	}
	rest := h[i+2:]
	if end := strings.IndexAny(rest, ", "); end >= 0 {
		rest = rest[:end]
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return n
}
