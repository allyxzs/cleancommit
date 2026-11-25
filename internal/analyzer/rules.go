package analyzer

import (
	"regexp"
	"strings"
)

// lineRule procura um padrão nas linhas ADICIONADAS do diff.
type lineRule struct {
	ID       string
	Category string
	Severity Severity
	Re       *regexp.Regexp
	Exts     map[string]bool  // nil = qualquer extensão
	Paths    []*regexp.Regexp // vazio = qualquer caminho (regras personalizadas)
	Message  string
	CodeOnly bool // ignora documentação e arquivos gerados
}

func newRule(id, category string, sev Severity, pattern, exts, message string, codeOnly bool) lineRule {
	r := lineRule{
		ID:       id,
		Category: category,
		Severity: sev,
		Re:       regexp.MustCompile(pattern),
		Message:  message,
		CodeOnly: codeOnly,
	}
	if exts != "" {
		r.Exts = map[string]bool{}
		for _, e := range strings.Split(exts, ",") {
			r.Exts[e] = true
		}
	}
	return r
}

func (r *lineRule) applies(path string) bool {
	if len(r.Paths) > 0 && !matchAny(r.Paths, path) {
		return false
	}
	if r.Exts != nil && !r.Exts[fileExt(path)] {
		return false
	}
	return true
}

func fileExt(path string) string {
	i := strings.LastIndex(path, ".")
	j := strings.LastIndex(path, "/")
	if i < 0 || i < j {
		return ""
	}
	return strings.ToLower(path[i+1:])
}

func builtinLineRules() []lineRule {
	const js = "js,jsx,ts,tsx,mjs,cjs,vue,svelte"
	return []lineRule{
		// Conflitos e segredos: valem para qualquer tipo de arquivo.
		newRule("conflict-marker", "conflict", Error, `^(<{7}|>{7})(\s|$)`, "", "Marcador de conflito de merge não resolvido", false),
		newRule("aws-access-key", "secret", Error, `\bAKIA[0-9A-Z]{16}\b`, "", "Possível chave de acesso da AWS", false),
		newRule("private-key", "secret", Error, `-----BEGIN ([A-Z]+ )*PRIVATE KEY-----`, "", "Chave privada dentro do código", false),
		newRule("github-token", "secret", Error, `\bgh[pousr]_[A-Za-z0-9]{36,}\b`, "", "Possível token do GitHub", false),
		newRule("slack-token", "secret", Error, `\bxox[abprs]-[A-Za-z0-9-]{10,}`, "", "Possível token do Slack", false),
		newRule("hardcoded-secret", "secret", Warning, `(?i)\b\w*(api[_-]?key|secret|passw(or)?d|token)\w*["']?\s*[:=]\s*["'][^"'\s]{8,}["']`, "", "Possível segredo fixo no código", false),

		// Código de depuração.
		newRule("js-console", "debug", Warning, `\bconsole\.(log|debug|trace|dir|table)\s*\(`, js, "console.* esquecido no código", true),
		newRule("js-debugger", "debug", Error, `^\s*debugger\s*;?\s*$`, js, "Instrução debugger esquecida", true),
		newRule("js-focused-test", "debug", Error, `\b(describe|it|test)\.only\s*\(`, js, "Teste focado (.only): os demais serão ignorados", false),
		newRule("py-breakpoint", "debug", Error, `\b(breakpoint\s*\(\s*\)|i?pdb\.set_trace\s*\(\s*\)|import\s+i?pdb\b)`, "py", "Breakpoint do depurador Python", true),
		newRule("py-print", "debug", Info, `^\s*print\s*\(`, "py", "print() adicionado: é log de depuração?", true),
		newRule("go-spew", "debug", Warning, `\bspew\.(Dump|Fdump|Sdump)\s*\(`, "go", "spew.Dump esquecido no código", true),
		newRule("go-print-stack", "debug", Warning, `\bdebug\.PrintStack\s*\(`, "go", "debug.PrintStack esquecido no código", true),
		newRule("rust-dbg", "debug", Error, `\bdbg!\s*\(`, "rs", "Macro dbg! esquecida", true),
		newRule("rust-todo", "debug", Warning, `\b(todo|unimplemented)!\s*\(`, "rs", "todo!/unimplemented! vai dar panic em runtime", true),
		newRule("java-sysout", "debug", Warning, `\bSystem\.(out|err)\.print(ln|f)?\s*\(`, "java,kt,scala", "System.out/err esquecido no código", true),
		newRule("print-stacktrace", "debug", Warning, `\.printStackTrace\s*\(\s*\)`, "java,kt,scala", "printStackTrace() esquecido no código", true),
		newRule("php-dump", "debug", Warning, `\b(var_dump|print_r|dd|ray)\s*\(`, "php", "Função de dump/depuração PHP", true),
		newRule("ruby-debugger", "debug", Error, `\b(binding\.(pry|irb)|byebug|debugger)\b`, "rb,erb", "Breakpoint do depurador Ruby", true),
		newRule("debug-print", "debug", Warning, `(?i)\b(print|println|printf|echo|puts|console\.log|log)\b\W{0,3}["'](debug|xxx|here|aqui|teste?)\b`, "", "Print de depuração (\"debug\", \"here\"...)", true),

		// Marcadores pendentes.
		newRule("todo-marker", "marker", Info, `\b(TODO|FIXME|HACK|XXX)\b`, "", "Marcador pendente (TODO/FIXME/HACK) adicionado", true),
	}
}

// fileSpec descreve arquivos que normalmente não deveriam ser commitados.
type fileSpec struct {
	ID       string
	Severity Severity
	Patterns []string
	Except   []string
	Message  string
}

var builtinFileSpecs = []fileSpec{
	{
		ID: "env-file", Severity: Error,
		Patterns: []string{".env", ".env.*"},
		Except:   []string{".env.example", ".env.sample", ".env.template", ".env.dist", ".env.tpl"},
		Message:  "Arquivo de ambiente costuma conter segredos",
	},
	{
		ID: "private-key-file", Severity: Error,
		Patterns: []string{"*.pem", "*.key", "*.p12", "*.pfx", "*.jks", "*.keystore", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"},
		Message:  "Chave privada ou certificado não deve ir para o repositório",
	},
	{
		ID: "dependency-dir", Severity: Warning,
		Patterns: []string{"node_modules/", "vendor/bundle/", ".venv/", "venv/", "__pycache__/", ".pytest_cache/", ".mypy_cache/", "*.egg-info/", ".gradle/"},
		Message:  "Diretório de dependências ou cache gerado automaticamente",
	},
	{
		ID: "build-output", Severity: Warning,
		Patterns: []string{"dist/", "build/", "target/", "coverage/", ".next/", ".nuxt/", ".parcel-cache/", ".turbo/"},
		Message:  "Artefato de build/saída gerado",
	},
	{
		ID: "compiled-file", Severity: Warning,
		Patterns: []string{"*.pyc", "*.pyo", "*.class", "*.o", "*.obj", "*.exe", "*.dll", "*.so", "*.dylib"},
		Message:  "Arquivo compilado ou binário gerado",
	},
	{
		ID: "os-junk", Severity: Warning,
		Patterns: []string{".DS_Store", "Thumbs.db", "desktop.ini", "._*"},
		Message:  "Arquivo criado pelo sistema operacional",
	},
	{
		ID: "editor-junk", Severity: Warning,
		Patterns: []string{"*.swp", "*.swo", "*~", ".idea/", "*.iml"},
		Message:  "Arquivo de editor/IDE",
	},
	{
		ID: "backup-file", Severity: Warning,
		Patterns: []string{"*.orig", "*.rej", "*.bak", "*.tmp", "*.temp"},
		Message:  "Arquivo temporário ou de backup",
	},
	{
		ID: "log-file", Severity: Warning,
		Patterns: []string{"*.log", "npm-debug.log*", "yarn-error.log*"},
		Message:  "Arquivo de log",
	},
	{
		ID: "local-database", Severity: Warning,
		Patterns: []string{"*.sqlite", "*.sqlite3", "*.db"},
		Message:  "Banco de dados local",
	},
	{
		ID: "vscode-dir", Severity: Info,
		Patterns: []string{".vscode/"},
		Message:  "Configuração local do VS Code (ok se for compartilhada de propósito)",
	},
}

type fileRule struct {
	ID       string
	Severity Severity
	re       *regexp.Regexp
	except   []*regexp.Regexp
	Message  string
}

func builtinFileRules() ([]fileRule, error) {
	var out []fileRule
	for _, s := range builtinFileSpecs {
		exc, err := compileGlobs(s.Except)
		if err != nil {
			return nil, err
		}
		for _, p := range s.Patterns {
			re, err := GlobToRegexp(p)
			if err != nil {
				return nil, err
			}
			out = append(out, fileRule{ID: s.ID, Severity: s.Severity, re: re, except: exc, Message: s.Message})
		}
	}
	return out, nil
}

// scopeCatalog lista as regras de escopo/tamanho (para `cleancommit rules`).
var scopeCatalog = []RuleInfo{
	{ID: "large-file", Category: "file", Severity: Warning, Description: "Arquivo maior que limits.max_file_size_kb"},
	{ID: "big-commit-files", Category: "scope", Severity: Warning, Description: "Mais arquivos que limits.max_files"},
	{ID: "big-commit-lines", Category: "scope", Severity: Warning, Description: "Mais linhas alteradas que limits.max_lines"},
	{ID: "huge-file-change", Category: "scope", Severity: Warning, Description: "Um único arquivo com mais linhas que limits.max_file_lines"},
	{ID: "mixed-concerns", Category: "scope", Severity: Warning, Description: "Código, docs, CI, dependências... misturados no mesmo commit"},
	{ID: "scattered-changes", Category: "scope", Severity: Warning, Description: "Alterações espalhadas por muitos módulos"},
	{ID: "lockfile-only", Category: "scope", Severity: Warning, Description: "Lockfile alterado sem o manifesto correspondente"},
	{ID: "whitespace-only", Category: "scope", Severity: Warning, Description: "Arquivo com mudança apenas de espaços/indentação"},
	{ID: "mass-deletion", Category: "scope", Severity: Warning, Description: "Muitos arquivos removidos de uma vez"},
}

// Catalog devolve todas as regras embutidas.
func Catalog() []RuleInfo {
	var out []RuleInfo
	for _, r := range builtinLineRules() {
		out = append(out, RuleInfo{ID: r.ID, Category: r.Category, Severity: r.Severity, Description: r.Message})
	}
	for _, s := range builtinFileSpecs {
		out = append(out, RuleInfo{ID: s.ID, Category: "file", Severity: s.Severity, Description: s.Message})
	}
	out = append(out, scopeCatalog...)
	return out
}
