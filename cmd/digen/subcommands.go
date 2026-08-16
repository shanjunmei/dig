package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/generator"
	"github.com/shanjunmei/dig/internal/loader"
	"github.com/shanjunmei/dig/internal/logger"
	"github.com/shanjunmei/dig/internal/model"
	"github.com/shanjunmei/dig/internal/processor"

	"github.com/shanjunmei/dig/pkg/alias"
	"golang.org/x/tools/go/packages"
)

// aliasStrategyFor parses the configured alias type and exits on error.
func aliasStrategyFor(cfg *config.Config) alias.AliasStrategy {
	at, err := alias.ParseAliasType(cfg.AliasType)
	if err != nil {
		fail("%v", err)
	}
	return alias.NewAliasStrategy(at)
}

// buildProcessor wires a Processor with a loader and (stub) generator.
func buildProcessor(cfg *config.Config) (*processor.Processor, *loader.PackageLoader) {
	l := loader.NewPackageLoader()
	g := generator.NewGenerator(logger.NewLogger(cfg), cfg)
	p := processor.NewProcessor(l, g, logger.NewLogger(cfg), cfg)
	return p, l
}

// mustLoad loads packages or fails with a clean message.
func mustLoad(l *loader.PackageLoader, paths []string) ([]*packages.Package, map[string]*packages.Package) {
	pkgs, pkgMap, err := l.Load(paths)
	if err != nil {
		fail("%v", err)
	}
	return pkgs, pkgMap
}

// runInit scaffolds a di.go with the dig.Build entry point.
func runInit(args []string) {
	name := "di.go"
	for i, a := range args {
		if !strings.HasPrefix(a, "-") {
			name = a
			args = append(args[:i], args[i+1:]...)
			break
		}
	}
	if _, err := os.Stat(name); err == nil {
		fail("file %s already exists (refusing to overwrite)", name)
	}
	if err := os.WriteFile(name, []byte(initTemplate), 0644); err != nil {
		fail("failed to write %s: %v", name, err)
	}
	fmt.Printf("[digen] created %s\n  run `digen` to generate wiring, or `go generate ./...`\n", name)
}

const initTemplate = `//go:build digen

package main

import (
	"context"

	"github.com/shanjunmei/dig"
)

// InitApp wires the application's dependencies at compile time.
// Run 'digen' (or 'go generate ./...') to (re)generate the wiring in dig_gen.go.
func InitApp() func(context.Context) error {
	return dig.Build(
		// Register constructors with dig.Provide:
		// dig.Provide(NewConfig),
		// dig.Provide(NewDB),

		// Consume the graph with dig.Invoke:
		// dig.Invoke(func(db *DB) {}),
	)
}
`

// runCheck validates DI contracts without writing any file.
func runCheck(f cliFlags, remaining []string) {
	cfg := buildConfig(f, remaining)
	p, l := buildProcessor(cfg)
	pkgs, pkgMap := mustLoad(l, cfg.Paths)
	checked := 0
	var failed []string
	for _, pkg := range pkgs {
		if err := p.CheckPackage(pkg, pkgMap, aliasStrategyFor(cfg)); err != nil {
			if strings.Contains(err.Error(), "no function containing dig.Build call found") {
				continue
			}
			failed = append(failed, fmt.Sprintf("  Package %s:\n    %s", pkg.PkgPath, err.Error()))
		} else {
			checked++
		}
	}
	if checked == 0 {
		if len(failed) > 0 {
			fail("%d package(s) found but failed validation:\n%s", len(failed), strings.Join(failed, "\n"))
		}
		fail("no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error")
	}
	if len(failed) > 0 {
		fmt.Printf("[digen] packages with issues:\n%s\n", strings.Join(failed, "\n"))
	}
	fmt.Printf("[digen] check passed: %d package(s)\n", checked)
}

// runGraph prints a Mermaid dependency graph per package.
func runGraph(f cliFlags, remaining []string) {
	cfg := buildConfig(f, remaining)
	p, l := buildProcessor(cfg)
	pkgs, pkgMap := mustLoad(l, cfg.Paths)
	strat := aliasStrategyFor(cfg)
	for _, pkg := range pkgs {
		nodes, err := p.ExtractNodes(pkg, pkgMap, strat)
		if err != nil {
			if strings.Contains(err.Error(), "no function containing dig.Build call found") {
				continue
			}
			fail("package %s: %v", pkg.PkgPath, err)
		}
		if len(nodes) == 0 {
			continue
		}
		fmt.Printf("# package %s\n", pkg.PkgPath)
		fmt.Println(renderMermaid(nodes))
	}
}

// renderMermaid renders the provider dependency graph as a Mermaid flowchart.
func renderMermaid(nodes []model.Node) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	nameToID := make(map[string]string)
	for i, n := range nodes {
		id := fmt.Sprintf("n%d", i)
		if n.Name != "" {
			nameToID[n.Name] = id
		}
		label := n.Name
		if n.IsInvoke {
			label = "Invoke " + label
		}
		if n.RetType != "" {
			label += " : " + n.RetType
		}
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, escapeMermaid(label))
	}
	for i, n := range nodes {
		for _, arg := range n.Args {
			if arg.IsConst || arg.IsContext {
				continue
			}
			tid, ok := nameToID[arg.Name]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "  %s --> n%d\n", tid, i)
		}
	}
	return b.String()
}

func escapeMermaid(s string) string {
	return strings.ReplaceAll(s, `"`, "&quot;")
}

// runExplain prints how a type/provider is resolved.
func runExplain(f cliFlags, remaining []string) {
	if len(remaining) == 0 {
		fail("explain requires a type or provider name, e.g. `digen explain DB ./...`")
	}
	query := remaining[0]
	paths := remaining[1:]
	if len(paths) == 0 {
		paths = []string{"."}
	}
	cfg := buildConfig(f, paths)
	p, l := buildProcessor(cfg)
	pkgs, pkgMap := mustLoad(l, cfg.Paths)
	strat := aliasStrategyFor(cfg)

	var all []model.Node
	for _, pkg := range pkgs {
		nodes, err := p.ExtractNodes(pkg, pkgMap, strat)
		if err != nil {
			if strings.Contains(err.Error(), "no function containing dig.Build call found") {
				continue
			}
			fail("package %s: %v", pkg.PkgPath, err)
		}
		all = append(all, nodes...)
	}
	if len(all) == 0 {
		fail("no packages with dig.Build found\n  💡 Fix: create a function with dig.Build(...) that returns func(context.Context) error")
	}

	var roots []model.Node
	for _, n := range all {
		if matchesQuery(n, query) {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		fail("no provider found for %q\n  💡 Tip: match by provider name or return type; try a shorter suffix", query)
	}
	for _, r := range roots {
		fmt.Printf("Resolution of %s (%s):\n", r.Name, r.RetType)
		printResolution(r, all, map[string]bool{}, 0)
	}
}

func matchesQuery(n model.Node, query string) bool {
	if n.Name == query || n.RetType == query {
		return true
	}
	return strings.HasSuffix(n.RetType, query) || strings.HasSuffix(n.RetType, "*"+query)
}

func printResolution(n model.Node, all []model.Node, seen map[string]bool, depth int) {
	if seen[n.Name] {
		fmt.Printf("%s↺ %s (already shown)\n", indent(depth), n.Name)
		return
	}
	seen[n.Name] = true
	prefix := "● "
	if depth > 0 {
		prefix = "└─ "
	}
	fmt.Printf("%s%s%s -> %s\n", indent(depth), prefix, n.Name, n.RetType)
	for _, arg := range n.Args {
		if arg.IsContext {
			continue
		}
		if arg.IsConst {
			fmt.Printf("%s   • %s (const %s)\n", indent(depth), arg.Name, arg.ConstValue)
			continue
		}
		provider, ok := findProvider(all, arg.Name)
		if !ok {
			fmt.Printf("%s   • %s (external)\n", indent(depth), arg.Name)
			continue
		}
		printResolution(provider, all, seen, depth+1)
	}
}

func findProvider(all []model.Node, name string) (model.Node, bool) {
	for _, n := range all {
		if n.Name == name {
			return n, true
		}
	}
	return model.Node{}, false
}

func indent(d int) string {
	return strings.Repeat("  ", d)
}

// runCompletion prints a shell completion script.
func runCompletion(args []string) {
	shell := "bash"
	if len(args) > 0 {
		shell = args[0]
	}
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		fail("unsupported shell %q (want bash, zsh, or fish)", shell)
	}
}

const bashCompletion = `# digen bash completion
_digen_complete() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    local subcommands="init check graph explain completion"
    local flags="-out -unused -debug -alias -inline -typecheck -cache -cachedir -version -h"
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$subcommands $flags" -- "$cur") )
        return
    fi
    local cmd="${COMP_WORDS[1]}"
    case "$cmd" in
        completion) COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") ) ;;
        *) COMPREPLY=( $(compgen -W "$flags" -- "$cur") ) ;;
    esac
}
complete -F _digen_complete digen
`

const zshCompletion = `#compdef digen
_digen() {
  local -a subcommands flags
  subcommands=(
    'init:scaffold a di.go'
    'check:validate DI contracts without writing'
    'graph:print a Mermaid dependency graph'
    'explain:explain how a type is resolved'
    'completion:print a shell completion script'
  )
  flags=(
    '-out:output file name'
    '-unused:error|ignore|drop'
    '-debug:enable debug logging'
    '-alias:full|short|obfuscated|numeric'
    '-inline:inline simple closures'
    '-typecheck:type-check generated code'
    '-cache:cache the extracted IR'
    '-cachedir:IR cache directory'
    '-version:print version and exit'
    '-h:show help'
  )
  if (( CURRENT == 2 )); then
    _describe 'command' subcommands
    _arguments $flags
  else
    case ${words[2]} in
      completion) _values 'shell' bash zsh fish ;;
      *) _arguments $flags ;;
    esac
  fi
}
_digen "$@"
`

const fishCompletion = `# digen fish completion
complete -c digen -n '__fish_use_subcommand' -a init -d 'scaffold a di.go'
complete -c digen -n '__fish_use_subcommand' -a check -d 'validate without writing'
complete -c digen -n '__fish_use_subcommand' -a graph -d 'print dependency graph'
complete -c digen -n '__fish_use_subcommand' -a explain -d 'explain type resolution'
complete -c digen -n '__fish_use_subcommand' -a completion -d 'print shell completion'
complete -c digen -n 'not __fish_use_subcommand' -l out -d 'output file name'
complete -c digen -n 'not __fish_use_subcommand' -l unused -x -a 'error ignore drop'
complete -c digen -n 'not __fish_use_subcommand' -l alias -x -a 'full short obfuscated numeric'
complete -c digen -n 'not __fish_use_subcommand' -l debug -d 'enable debug logging'
complete -c digen -n 'not __fish_use_subcommand' -l inline -d 'inline simple closures'
complete -c digen -n 'not __fish_use_subcommand' -l typecheck -d 'type-check generated code'
complete -c digen -n 'not __fish_use_subcommand' -l cache -d 'cache the extracted IR'
complete -c digen -n 'not __fish_use_subcommand' -l cachedir -d 'IR cache directory'
complete -c digen -n 'not __fish_use_subcommand' -l version -d 'print version and exit'

`
