package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/shanjunmei/dig/internal/config"
	"github.com/shanjunmei/dig/internal/model"
)

// cliFlags holds the parsed command-line flags. It is built once in main and
// passed explicitly to the subcommands, so no flag value lives at package
// scope. This matters because cmd/digen is the framework's own example: we
// don't want to teach package-level global state to readers who copy it.
type cliFlags struct {
	out       string
	unused    string
	debug     bool
	alias     string
	inline    bool
	typecheck bool
	cache     bool
	cachedir  string
}

func main() {
	fs := flag.NewFlagSet("digen", flag.ExitOnError)

	var (
		out       = fs.String("out", "dig_gen.go", "output file name (ignored in ./... mode)")
		unused    = fs.String("unused", "error", "behavior for unused providers: error, ignore, drop")
		debug     = fs.Bool("debug", false, "enable debug logging")
		alias     = fs.String("alias", "full", "import-alias strategy: full, short, obfuscated, numeric")
		showVer   = fs.Bool("version", false, "print version information and exit")
		inline    = fs.Bool("inline", false, "inline simple closures as IIFEs; identity closures collapse to a type conversion")
		typecheck = fs.Bool("typecheck", true, "type-check generated code to catch internal generator bugs (disable for large ./... runs)")
		cache     = fs.Bool("cache", false, "cache the extracted IR to disk and reuse it for unchanged packages (skips extraction on cache hit)")
		cachedir  = fs.String("cachedir", "", "IR cache directory (default: os.TempDir()/digen-ir-cache); ignored unless -cache is set")
	)

	fs.Usage = func() {
		out := os.Stderr
		fmt.Fprintf(out, "Usage: digen [options] <packages...>\n\n")
		fmt.Fprintf(out, "digen generates dig_gen.go (compile-time DI wiring) for the given Go packages.\n")
		fmt.Fprintf(out, "If no package is given, the current directory (\".\") is used.\n\n")
		fmt.Fprintf(out, "Commands:\n")
		fmt.Fprintf(out, "  init                  scaffold a di.go with the dig.Build entry point\n")
		fmt.Fprintf(out, "  check [pkgs]          validate DI contracts without writing files\n")
		fmt.Fprintf(out, "  graph [pkgs]          print a Mermaid dependency graph of the providers\n")
		fmt.Fprintf(out, "  explain <type> [pkgs] explain how a type/provider is resolved\n")
		fmt.Fprintf(out, "  completion <shell>    print shell completion (bash, zsh, fish)\n\n")
		fmt.Fprintf(out, "Examples:\n")
		fmt.Fprintf(out, "  digen ./...                         # generate for every package in the module\n")
		fmt.Fprintf(out, "  digen -inline ./...                 # also inline simple closures as IIFEs\n")
		fmt.Fprintf(out, "  digen -cache ./...                  # cache the extracted IR for faster re-runs\n")
		fmt.Fprintf(out, "  digen -unused=ignore .              # don't error on unused providers\n")
		fmt.Fprintf(out, "  digen check ./...                   # validate without writing\n")
		fmt.Fprintf(out, "  digen graph ./...                   # print dependency graph (Mermaid)\n")
		fmt.Fprintf(out, "  digen explain DB ./...              # explain how *DB is resolved\n\n")
		fmt.Fprintf(out, "Options:\n")
		fs.PrintDefaults()
	}

	fs.Parse(os.Args[1:])

	if *showVer {
		fmt.Println(versionString())
		return
	}

	f := cliFlags{
		out:       *out,
		unused:    *unused,
		debug:     *debug,
		alias:     *alias,
		inline:    *inline,
		typecheck: *typecheck,
		cache:     *cache,
		cachedir:  *cachedir,
	}

	args := fs.Args()
	if len(args) > 0 {
		switch args[0] {
		case "init":
			runInit(args[1:])
			return
		case "check":
			runCheck(f, args[1:])
			return
		case "graph":
			runGraph(f, args[1:])
			return
		case "explain":
			runExplain(f, args[1:])
			return
		case "completion":
			runCompletion(args[1:])
			return
		case "help", "-help", "--help":
			fs.Usage()
			return
		}
	}

	cfg := buildConfig(f, args)
	run := InitApp(cfg)
	if err := run(context.Background()); err != nil {
		fail("application error: %v", err)
	}
}

// buildConfig turns the parsed flags + positional package paths into a Config.
func buildConfig(f cliFlags, paths []string) *config.Config {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return &config.Config{
		OutputFile:     f.out,
		UnusedMode:     parseUnusedMode(&f.unused),
		Debug:          f.debug,
		AliasType:      f.alias,
		Paths:          paths,
		InlineClosures: f.inline,
		TypeCheckNet:   f.typecheck,
		Cache:          f.cache,
		CacheDir:       f.cachedir,
	}
}

func parseUnusedMode(s *string) model.UnusedMode {
	switch *s {
	case "ignore":
		return model.UnusedModeIgnore
	case "drop":
		return model.UnusedModeDrop
	case "error":
		return model.UnusedModeError
	}
	fail("invalid -unused value %q, allowed: error, ignore, drop", *s)
	panic("unreachable") // fail() exits via os.Exit(1); control never reaches here
}
