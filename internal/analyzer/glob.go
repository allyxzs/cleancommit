package analyzer

import (
	"regexp"
	"strings"
)

// GlobToRegexp converte um padrão estilo .gitignore em expressão regular.
//
//	*.log          qualquer arquivo .log, em qualquer pasta
//	node_modules/  qualquer pasta com esse nome (e tudo dentro dela)
//	docs/legacy/** caminho a partir da raiz do repositório
//	src/**/*.go    ** casa zero ou mais pastas
func GlobToRegexp(pattern string) (*regexp.Regexp, error) {
	p := strings.TrimSpace(pattern)
	p = strings.ReplaceAll(p, "\\", "/")
	dirOnly := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	anchored := strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")

	var b strings.Builder
	if anchored {
		b.WriteString("^")
	} else {
		b.WriteString("(?:^|.*/)")
	}
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				if i+2 < len(p) && p[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(p[i : i+1]))
		}
	}
	if dirOnly {
		b.WriteString("/.*")
	} else {
		b.WriteString("$")
	}
	return regexp.Compile(b.String())
}

func compileGlobs(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := GlobToRegexp(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
