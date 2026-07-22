// Command alloyctl is the CLI entrypoint for Alloy (phase 1 of 3 — TUI
// and GUI phases will reuse the internal/ packages this talks to,
// unchanged).
//
// NOTE ON CLI LIBRARY: the spec calls for spf13/cobra. This dev
// sandbox's network egress allowlist doesn't include gopkg.in, which
// cobra pulls in transitively (via a doc-generation dependency), so
// `go get` for it fails here. Rather than ship something we couldn't
// actually verify builds, this hand-rolls a small subcommand dispatcher
// on the stdlib flag package instead — the command surface (verbs,
// flags, help text) is identical to what a cobra-based version would
// expose, so swapping the library in later, in an environment that can
// reach gopkg.in, is a mechanical change confined to this file.
package main

import (
	"fmt"
	"os"

	"alloy/internal/cli"
	"alloy/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		cli.Errorf("%s", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	paths, err := config.Resolve()
	if err != nil {
		return fmt.Errorf("resolving app directories: %w", err)
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "auth":
		return cmdAuth(paths, rest)
	case "install":
		return cmdInstall(paths, rest)
	case "play":
		return cmdPlay(paths, rest)
	case "rename":
		return cmdRename(paths, rest)
	case "remove":
		return cmdRemove(paths, rest)
	case "java":
		return cmdJava(paths, rest)
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func printUsage() {
	bold := "\033[1m"
	cyan := "\033[36m"
	reset := "\033[0m"

	art := bold +
		"        ######### #########        \n" +
		"        ######### #########        \n" +
		"        ############ ######        \n" +
		"        ############ ######        \n" +
		"        ######### #########        \n" +
		"        ######### #########        " + reset

	fmt.Println(art)
	fmt.Println()
	fmt.Println(" ", bold+"alloyctl"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"Usage:"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"auth"+reset)
	fmt.Println("   ", cyan+"online"+reset)
	fmt.Println("   ", cyan+"offline <username>"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"install"+reset)
	fmt.Println("   ", cyan+"<version> [--fabric|--quilt|--forge|--neoforge]"+reset)
	fmt.Println("             ", cyan+"[--name <instance>]"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"play"+reset)
	fmt.Println("   ", cyan+"[instance]"+reset)
	fmt.Println("             ", cyan+"[--memory <MB>] [--width <px>] [--height <px>]"+reset)
	fmt.Println("             ", cyan+"[--jvm <arg>] (repeatable, e.g. --jvm \"-Xss4m\")"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"manage"+reset)
	fmt.Println("   ", cyan+"rename <old> <new>"+reset)
	fmt.Println("   ", cyan+"remove <instance>"+reset)
	fmt.Println()
	fmt.Println(" ", bold+"java"+reset)
	fmt.Println("   ", cyan+"list"+reset)
	fmt.Println("   ", cyan+"set <path>"+reset)
}
