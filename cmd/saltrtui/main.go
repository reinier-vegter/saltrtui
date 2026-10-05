package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/app"
	"saltrtui/internal/saltcli"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("saltrtui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "Salt master configuration directory (default: Salt's own lookup)")
	noAltScreen := flags.Bool("no-alt-screen", false, "render in the current terminal screen")
	showVersion := flags.Bool("version", false, "print the installed version and exit")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "saltrtui %s\n", version)
		return 0
	}
	backend := saltcli.New(*configDir)
	model := app.NewWithHighstate(backend, backend, backend, backend, backend, backend, backend, *configDir, !*noAltScreen)
	defer model.Close()
	result, err := tea.NewProgram(model).Run()
	if stopped, ok := result.(app.Model); ok {
		stopped.Close()
	}
	if err != nil {
		fmt.Fprintf(stderr, "saltrtui: %v\n", err)
		return 1
	}
	return 0
}
