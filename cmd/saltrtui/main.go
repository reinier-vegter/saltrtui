package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/app"
	"saltrtui/internal/cache"
	"saltrtui/internal/release"
	"saltrtui/internal/saltcli"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 5 && args[0] == "--install-stdin" {
		length, err := strconv.ParseInt(args[4], 10, 64)
		if err != nil || release.InstallFromReader(args[1], args[2], args[3], length, os.Stdin) != nil {
			fmt.Fprintln(stderr, "saltrtui: installation failed")
			return 1
		}
		return 0
	}
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
	updateStore, err := cache.NewStore()
	if err != nil {
		updateStore = nil
	}
	model := app.NewWithUpdates(backend, backend, backend, backend, backend, backend, backend, *configDir, !*noAltScreen, version, updateStore)
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
