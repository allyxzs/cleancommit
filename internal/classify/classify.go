// Package classify agrupa arquivos por natureza (código, testes, docs, CI...).
package classify

import (
	pathpkg "path"
	"strings"
)

// Kind é a natureza de um arquivo.
type Kind string

const (
	Code      Kind = "code"
	Test      Kind = "test"
	Docs      Kind = "docs"
	CI        Kind = "ci"
	Deps      Kind = "deps"
	Build     Kind = "build"
	Config    Kind = "config"
	Asset     Kind = "asset"
	Generated Kind = "generated"
)

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

var lockToManifest = map[string]string{
	"package-lock.json":   "package.json",
	"npm-shrinkwrap.json": "package.json",
	"yarn.lock":           "package.json",
	"pnpm-lock.yaml":      "package.json",
	"bun.lockb":           "package.json",
	"Cargo.lock":          "Cargo.toml",
	"go.sum":              "go.mod",
	"poetry.lock":         "pyproject.toml",
	"Pipfile.lock":        "Pipfile",
	"Gemfile.lock":        "Gemfile",
	"composer.lock":       "composer.json",
}

var (
	docExts    = set("md", "mdx", "rst", "txt", "adoc", "org")
	configExts = set("json", "yaml", "yml", "toml", "ini", "cfg", "conf", "env")
	assetExts  = set("png", "jpg", "jpeg", "gif", "svg", "ico", "webp", "woff", "woff2", "ttf", "eot", "mp4", "mp3", "pdf")
	depsFiles  = set("package.json", "go.mod", "cargo.toml", "pyproject.toml", "pipfile", "gemfile", "composer.json", "pom.xml", "build.gradle", "build.gradle.kts", "setup.py", "setup.cfg")
	buildFiles = set("makefile", "cmakelists.txt", ".dockerignore", "justfile")
	wrapperDir = set("src", "internal", "pkg", "cmd", "app", "apps", "lib", "libs", "packages", "crates", "services", "modules")
)

// IsLockfile informa se o arquivo é um lockfile conhecido.
func IsLockfile(p string) bool {
	_, ok := lockToManifest[pathpkg.Base(p)]
	return ok
}

// Manifest devolve o manifesto correspondente a um lockfile (mesmo diretório).
func Manifest(p string) (string, bool) {
	m, ok := lockToManifest[pathpkg.Base(p)]
	if !ok {
		return "", false
	}
	dir := pathpkg.Dir(p)
	if dir == "." {
		return m, true
	}
	return dir + "/" + m, true
}

// IsGenerated informa se o arquivo parece gerado por ferramenta.
func IsGenerated(p string) bool {
	if IsLockfile(p) {
		return true
	}
	lower := strings.ToLower(p)
	for _, suf := range []string{".min.js", ".min.css", ".map", ".snap", ".pb.go", ".pb.cc", ".pb.h", "_pb2.py", ".g.dart", ".lock"} {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return strings.HasPrefix(lower, "generated/") || strings.Contains(lower, "/generated/")
}

func hasDir(dirs []string, names ...string) bool {
	for _, d := range dirs {
		for _, n := range names {
			if d == n {
				return true
			}
		}
	}
	return false
}

// Of classifica um arquivo pelo caminho.
func Of(p string) Kind {
	if IsGenerated(p) {
		return Generated
	}
	lower := strings.ToLower(p)
	base := pathpkg.Base(lower)
	ext := strings.TrimPrefix(pathpkg.Ext(base), ".")
	segs := strings.Split(lower, "/")
	dirs := segs[:len(segs)-1]

	switch {
	case strings.HasPrefix(lower, ".github/workflows/"), strings.HasPrefix(lower, ".circleci/"),
		base == ".gitlab-ci.yml", base == "jenkinsfile", base == ".travis.yml", base == "azure-pipelines.yml":
		return CI
	case depsFiles[base], strings.HasPrefix(base, "requirements") && ext == "txt":
		return Deps
	case buildFiles[base], strings.HasPrefix(base, "dockerfile"), strings.HasPrefix(base, "docker-compose"):
		return Build
	case strings.Contains(base, "_test."), strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.HasPrefix(base, "test_"), hasDir(dirs, "test", "tests", "__tests__", "spec", "specs", "e2e"):
		return Test
	case docExts[ext], hasDir(dirs, "docs", "doc"),
		strings.HasPrefix(base, "readme"), strings.HasPrefix(base, "license"),
		strings.HasPrefix(base, "changelog"), strings.HasPrefix(base, "contributing"):
		return Docs
	case configExts[ext], strings.HasPrefix(base, "."):
		return Config
	case assetExts[ext]:
		return Asset
	}
	return Code
}

// Module devolve o "módulo" de um arquivo: o primeiro diretório, ou os dois
// primeiros quando o primeiro é só um agrupador (src/, internal/, cmd/...).
// Arquivos na raiz (ou diretamente num agrupador) devolvem "".
func Module(p string) string {
	segs := strings.Split(p, "/")
	if len(segs) < 2 {
		return ""
	}
	first := segs[0]
	if wrapperDir[strings.ToLower(first)] {
		if len(segs) > 2 {
			return first + "/" + segs[1]
		}
		return ""
	}
	return first
}
