// CleanCommit — revisor inteligente de alterações Git.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cleancommit/internal/analyzer"
	"cleancommit/internal/config"
	"cleancommit/internal/git"
	"cleancommit/internal/hooks"
	"cleancommit/internal/report"
	"cleancommit/internal/suggest"
)

// version pode ser sobrescrita no build: -ldflags "-X main.version=1.2.3"
var version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return cmdCheck(nil)
	}
	switch args[0] {
	case "check":
		return cmdCheck(args[1:])
	case "suggest", "msg":
		return cmdSuggest(args[1:])
	case "hook":
		return cmdHook(args[1:])
	case "hook-msg":
		return cmdHookMsg(args[1:])
	case "init":
		return cmdInit(args[1:])
	case "rules":
		return cmdRules(args[1:])
	case "version", "--version", "-v":
		fmt.Println("cleancommit " + version)
		return 0
	case "help", "--help", "-h":
		usage(os.Stdout)
		return 0
	}
	if strings.HasPrefix(args[0], "-") {
		return cmdCheck(args)
	}
	fmt.Fprintf(os.Stderr, "cleancommit: comando desconhecido %q\n\n", args[0])
	usage(os.Stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprint(w, `CleanCommit — revisa suas alterações antes de virarem commit.

Uso:
  cleancommit [comando] [opções]

Comandos:
  check            Revisa o que está no stage (padrão)
  suggest          Sugere uma mensagem de commit (Conventional Commits)
  hook             Gerencia os Git hooks: install | uninstall | status
  init             Cria um .cleancommit.json de exemplo
  rules            Lista as regras embutidas
  version          Mostra a versão

Opções do check:
  --working        Analisa o diretório de trabalho (vs HEAD) em vez do stage
  --json           Saída em JSON
  --fail-on NÍVEL  error (padrão), warning, info ou never
  --config CAMINHO Usa outro arquivo de configuração
  --no-color       Desativa cores

Opções do suggest:
  --plain          Imprime só a mensagem (ótimo para scripts)
  --commit         Faz o commit com a mensagem sugerida
  --lang pt|en     Idioma do assunto

Variáveis de ambiente:
  CLEANCOMMIT_SKIP=1   pula a checagem do hook
  NO_COLOR             desativa cores

Exemplos:
  cleancommit
  cleancommit suggest --plain
  cleancommit hook install --with-msg
`)
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "cleancommit: "+err.Error())
	return 2
}

// parse devolve (código, parar). parar=true quando o programa deve encerrar já.
func parse(fs *flag.FlagSet, args []string) (int, bool) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, true
		}
		return 2, true
	}
	return 0, false
}

type env struct {
	repo *git.Repo
	cfg  *config.Config
}

func setup(cfgPath string) (*env, error) {
	repo, err := git.Open()
	if err != nil {
		return nil, err
	}
	path := cfgPath
	if path == "" {
		path = config.Discover(repo.Root)
	}
	cfg := config.Default()
	if path != "" {
		cfg, err = config.Load(path)
		if err != nil {
			return nil, err
		}
	}
	return &env{repo: repo, cfg: cfg}, nil
}

type jsonOutput struct {
	Tool             string             `json:"tool"`
	Version          string             `json:"version"`
	Mode             string             `json:"mode"`
	Blocked          bool               `json:"blocked"`
	Summary          analyzer.Summary   `json:"summary"`
	Findings         []analyzer.Finding `json:"findings"`
	SuggestedMessage string             `json:"suggested_message,omitempty"`
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	working := fs.Bool("working", false, "analisa o diretório de trabalho (vs HEAD) em vez do stage")
	asJSON := fs.Bool("json", false, "saída em JSON")
	hook := fs.Bool("hook", false, "modo Git hook (usado pelo pre-commit)")
	noColor := fs.Bool("no-color", false, "desativa cores")
	cfgPath := fs.String("config", "", "caminho do arquivo de configuração")
	failOn := fs.String("fail-on", "", "nível que bloqueia: error, warning, info ou never")
	if code, stop := parse(fs, args); stop {
		return code
	}

	if *hook && os.Getenv("CLEANCOMMIT_SKIP") == "1" {
		return 0
	}
	e, err := setup(*cfgPath)
	if err != nil {
		return fail(err)
	}
	if *failOn != "" {
		lvl := strings.ToLower(*failOn)
		if !config.ValidFailOn(lvl) {
			return fail(fmt.Errorf("--fail-on inválido: %q (use error, warning, info ou never)", *failOn))
		}
		e.cfg.FailOn = lvl
	}

	mode, label := git.Staged, "em stage"
	if *working {
		mode, label = git.Working, "no diretório de trabalho"
	}
	files, err := e.repo.Changes(mode)
	if err != nil {
		return fail(err)
	}

	// Git redireciona o stdout dos hooks para o stderr.
	var out io.Writer = os.Stdout
	colorFile := os.Stdout
	if *hook {
		out, colorFile = os.Stderr, os.Stderr
	}
	st := report.Style{On: report.ColorEnabled(*noColor, colorFile)}

	if len(files) == 0 && !*asJSON {
		if !*hook {
			fmt.Fprintln(out, "Nada para revisar: nenhuma alteração "+label+".")
		}
		return 0
	}

	an, err := analyzer.New(e.cfg)
	if err != nil {
		return fail(err)
	}
	res := an.Analyze(files)
	blocked := res.Blocked(e.cfg.FailOn)

	if *asJSON {
		payload := jsonOutput{
			Tool:     "cleancommit",
			Version:  version,
			Mode:     label,
			Blocked:  blocked,
			Summary:  res.Summary,
			Findings: res.Findings,
		}
		if s := suggest.Build(files, e.repo.Branch(), e.cfg.Language); s != nil {
			payload.SuggestedMessage = s.Message()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return fail(err)
		}
	} else {
		report.Render(out, res, st, report.Options{Mode: label, FailOn: e.cfg.FailOn, Blocked: blocked, Hook: *hook})
	}

	if blocked {
		return 1
	}
	return 0
}

func cmdSuggest(args []string) int {
	fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
	working := fs.Bool("working", false, "usa o diretório de trabalho (vs HEAD) em vez do stage")
	plain := fs.Bool("plain", false, "imprime só a mensagem")
	doCommit := fs.Bool("commit", false, "faz o commit com a mensagem sugerida")
	lang := fs.String("lang", "", "idioma do assunto: pt ou en")
	noColor := fs.Bool("no-color", false, "desativa cores")
	cfgPath := fs.String("config", "", "caminho do arquivo de configuração")
	if code, stop := parse(fs, args); stop {
		return code
	}

	e, err := setup(*cfgPath)
	if err != nil {
		return fail(err)
	}
	language := e.cfg.Language
	if *lang != "" {
		language = strings.ToLower(*lang)
		if language != "pt" && language != "en" {
			return fail(fmt.Errorf("--lang inválido: %q (use pt ou en)", *lang))
		}
	}

	mode := git.Staged
	if *working {
		mode = git.Working
	}
	files, err := e.repo.Changes(mode)
	if err != nil {
		return fail(err)
	}
	s := suggest.Build(files, e.repo.Branch(), language)
	if s == nil {
		fmt.Fprintln(os.Stderr, "Nada para sugerir: nenhuma alteração encontrada (use git add primeiro).")
		return 1
	}

	switch {
	case *plain:
		fmt.Println(s.Message())
	case *doCommit:
		if err := e.repo.Commit(s.Header(), s.Body, s.Footer); err != nil {
			return fail(err)
		}
	default:
		st := report.Style{On: report.ColorEnabled(*noColor, os.Stdout)}
		report.RenderSuggestion(os.Stdout, s, st)
	}
	return 0
}

func cmdHook(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "uso: cleancommit hook install|uninstall|status [--with-msg] [--force]")
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet("hook "+sub, flag.ContinueOnError)
	withMsg := fs.Bool("with-msg", false, "instala também o prepare-commit-msg (pré-preenche a mensagem sugerida)")
	force := fs.Bool("force", false, "anexa o bloco a um hook existente que não é do CleanCommit")
	noColor := fs.Bool("no-color", false, "desativa cores")
	if code, stop := parse(fs, args[1:]); stop {
		return code
	}

	repo, err := git.Open()
	if err != nil {
		return fail(err)
	}
	st := report.Style{On: report.ColorEnabled(*noColor, os.Stdout)}

	names := []string{hooks.PreCommit}
	if *withMsg || sub == "uninstall" || sub == "status" {
		names = append(names, hooks.PrepareCommitMsg)
	}

	switch sub {
	case "install":
		for _, name := range names {
			path, err := repo.HookPath(name)
			if err != nil {
				return fail(err)
			}
			action, err := hooks.Install(path, name, *force)
			if err != nil {
				return fail(err)
			}
			fmt.Printf(" %s %s %s %s\n", st.Green("✔"), st.Bold(name), action, st.Dim(relPath(repo.Root, path)))
		}
		fmt.Println(st.Dim("   Ignorar uma vez: git commit --no-verify  (ou CLEANCOMMIT_SKIP=1 git commit)"))
		if _, err := exec.LookPath("cleancommit"); err != nil {
			fmt.Println(st.Yellow("   Aviso: `cleancommit` não está no PATH; o hook não fará nada até que esteja."))
		}
	case "uninstall":
		for _, name := range names {
			path, err := repo.HookPath(name)
			if err != nil {
				return fail(err)
			}
			removed, err := hooks.Uninstall(path)
			if err != nil {
				return fail(err)
			}
			if removed {
				fmt.Printf(" %s %s removido\n", st.Green("✔"), st.Bold(name))
			} else {
				fmt.Printf(" %s %s não estava instalado\n", st.Dim("–"), name)
			}
		}
	case "status":
		for _, name := range names {
			path, err := repo.HookPath(name)
			if err != nil {
				return fail(err)
			}
			if hooks.Installed(path) {
				fmt.Printf(" %s %s instalado %s\n", st.Green("✔"), st.Bold(name), st.Dim(relPath(repo.Root, path)))
			} else {
				fmt.Printf(" %s %s não instalado\n", st.Dim("–"), name)
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "cleancommit: subcomando desconhecido %q (use install, uninstall ou status)\n", sub)
		return 2
	}
	return 0
}

// cmdHookMsg é chamado pelo hook prepare-commit-msg. Nunca bloqueia o commit.
func cmdHookMsg(args []string) int {
	if len(args) < 1 {
		return 0
	}
	file := args[0]
	source := ""
	if len(args) > 1 {
		source = args[1]
	}
	e, err := setup("")
	if err != nil {
		return 0
	}
	files, err := e.repo.Changes(git.Staged)
	if err != nil || len(files) == 0 {
		return 0
	}
	if s := suggest.Build(files, e.repo.Branch(), e.cfg.Language); s != nil {
		_ = hooks.PrefillMessage(file, source, s.Message())
	}
	return 0
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "sobrescreve o arquivo existente")
	if code, stop := parse(fs, args); stop {
		return code
	}
	repo, err := git.Open()
	if err != nil {
		return fail(err)
	}
	target := filepath.Join(repo.Root, config.FileName)
	if _, err := os.Stat(target); err == nil && !*force {
		return fail(fmt.Errorf("%s já existe (use --force para sobrescrever)", config.FileName))
	}
	if err := os.WriteFile(target, []byte(config.Sample()), 0o644); err != nil {
		return fail(err)
	}
	fmt.Println("✔ Criado " + config.FileName)
	return 0
}

func cmdRules(args []string) int {
	fs := flag.NewFlagSet("rules", flag.ContinueOnError)
	noColor := fs.Bool("no-color", false, "desativa cores")
	if code, stop := parse(fs, args); stop {
		return code
	}
	st := report.Style{On: report.ColorEnabled(*noColor, os.Stdout)}
	report.RenderRules(os.Stdout, analyzer.Catalog(), st)
	return 0
}

func relPath(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
