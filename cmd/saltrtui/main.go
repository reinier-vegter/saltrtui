package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/app"
	"saltrtui/internal/saltcli"
)

func main() {
	configDir := flag.String("config-dir", "", "Salt master configuration directory (default: Salt's own lookup)")
	noAltScreen := flag.Bool("no-alt-screen", false, "render in the current terminal screen")
	flag.Parse()
	backend := saltcli.New(*configDir)
	model := app.NewWithHighstate(backend, backend, backend, backend, backend, backend, backend, *configDir, !*noAltScreen)
	defer model.Close()
	result, err := tea.NewProgram(model).Run()
	if stopped, ok := result.(app.Model); ok {
		stopped.Close()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "saltrtui: %v\n", err)
		os.Exit(1)
	}
}
