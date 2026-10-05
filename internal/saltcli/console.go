package saltcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"

	"saltrtui/internal/console"
)

const historyBytes = 256 << 10

func (g *Gateway) command(ctx context.Context, id, line, cwd string, timeout int, shell bool) (console.Result, error) {
	if !validID(id) || strings.ContainsFunc(id, unicode.IsSpace) {
		return console.Result{}, errors.New("minion ID cannot be safely targeted as a single list item")
	}
	if line == "" || len(line) > 8192 || strings.ContainsRune(line, 0) {
		return console.Result{}, errors.New("command is empty or exceeds input limit")
	}
	if cwd != "" && (!path.IsAbs(cwd) || strings.ContainsFunc(cwd, unicode.IsControl)) {
		return console.Result{}, errors.New("invalid remote working directory")
	}
	args := []string{"-L", id, "cmd.run_all", "cmd=" + line, fmt.Sprintf("python_shell=%t", shell), "redirect_stderr=False", "rstrip=False", fmt.Sprintf("timeout=%d", timeout)}
	if shell {
		args = append(args, "shell=/bin/bash")
	}
	if cwd != "" {
		args = append(args, "cwd="+cwd)
	}
	args = append(args, "--timeout=40", "--static", "--out=json", "--no-color")
	data, err := g.invoke(ctx, "salt", args...)
	if err != nil {
		return console.Result{}, fmt.Errorf("minion command outcome unknown: %w", err)
	}
	var envelope map[string]json.RawMessage
	if err := decode(data, &envelope); err != nil || len(envelope) != 1 {
		return console.Result{}, errors.New("minion command: missing or unexpected Salt return")
	}
	value, ok := envelope[id]
	if !ok {
		return console.Result{}, errors.New("minion command: returned ID differs from requested minion")
	}
	var ret struct {
		Stdout  *string `json:"stdout"`
		Stderr  *string `json:"stderr"`
		Retcode *int    `json:"retcode"`
	}
	if err := decode(value, &ret); err != nil || ret.Stdout == nil || ret.Stderr == nil || ret.Retcode == nil {
		return console.Result{}, errors.New("minion command: invalid or missing return")
	}
	return console.Result{Stdout: *ret.Stdout, Stderr: *ret.Stderr, Retcode: *ret.Retcode}, nil
}

func (g *Gateway) Run(ctx context.Context, id, line, cwd string) (console.Result, error) {
	return g.command(ctx, id, line, cwd, 30, true)
}

func (g *Gateway) Identity(ctx context.Context, id string) (console.Identity, error) {
	account, err := g.command(ctx, id, "id -un", "", 5, false)
	if err != nil || account.Retcode != 0 {
		return console.Identity{}, errors.New("execution account probe failed")
	}
	name := strings.TrimSpace(account.Stdout)
	if name == "" || strings.ContainsFunc(name, unicode.IsSpace) {
		return console.Identity{}, errors.New("invalid execution account")
	}
	cwd, err := g.ChangeDirectory(ctx, id, "")
	if err != nil {
		return console.Identity{}, err
	}
	partial := console.Identity{Account: name, Cwd: cwd}
	data, err := g.invoke(ctx, "salt", "-L", id, "user.info", "name="+name, "--timeout=5", "--static", "--out=json", "--no-color")
	if err != nil {
		return partial, fmt.Errorf("account home lookup: %w", err)
	}
	var response map[string]json.RawMessage
	if err := decode(data, &response); err != nil || len(response) != 1 {
		return partial, errors.New("account home lookup: invalid Salt response")
	}
	value, ok := response[id]
	if !ok {
		return partial, errors.New("account home lookup: unexpected minion return")
	}
	var info struct {
		Home string `json:"home"`
	}
	if err := decode(value, &info); err != nil || !path.IsAbs(info.Home) || strings.ContainsFunc(info.Home, unicode.IsControl) {
		return partial, errors.New("account home lookup: invalid home directory")
	}
	return console.Identity{Account: name, Home: info.Home, Cwd: cwd}, nil
}

func (g *Gateway) ChangeDirectory(ctx context.Context, id, cwd string) (string, error) {
	result, err := g.command(ctx, id, "pwd", cwd, 5, false)
	if err != nil {
		return "", err
	}
	dir := strings.TrimSuffix(result.Stdout, "\n")
	if result.Retcode != 0 || !path.IsAbs(dir) || strings.ContainsFunc(dir, unicode.IsControl) || strings.ContainsRune(dir, '\n') {
		return "", errors.New("directory unavailable or not accessible")
	}
	return dir, nil
}

func (g *Gateway) History(ctx context.Context, id, home string) ([]string, error) {
	if !path.IsAbs(home) || strings.ContainsFunc(home, unicode.IsControl) {
		return nil, errors.New("invalid account home")
	}
	// Quote for Salt's shlex parsing; python_shell=False prevents shell
	// metacharacters in an account home from being evaluated.
	file := path.Join(home, ".bash_history")
	quoted := "'" + strings.ReplaceAll(file, "'", "'\"'\"'") + "'"
	result, err := g.command(ctx, id, fmt.Sprintf("tail -c %d -- %s", historyBytes, quoted), "", 8, false)
	if err != nil {
		return nil, err
	}
	if result.Retcode != 0 {
		return nil, errors.New("saved Bash history unavailable")
	}
	if len(result.Stdout) > historyBytes {
		return nil, errors.New("saved Bash history exceeds limit")
	}
	raw := result.Stdout
	if len(raw) == historyBytes {
		if index := strings.IndexByte(raw, '\n'); index >= 0 {
			raw = raw[index+1:]
		}
	}
	return console.ParseHistory(raw), nil
}
