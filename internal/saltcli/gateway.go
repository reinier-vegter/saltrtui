// Package saltcli implements the local-master Salt gateway.
package saltcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"saltrtui/internal/fleet"
	"saltrtui/internal/jobs"
)

const outputLimit = 4 << 20

var jidPattern = regexp.MustCompile(`^[0-9]{14,24}$`)

// Salt 3008.3 can emit Python object-cleanup warnings after a runner returns
// valid JSON. Permit only the observed complete lines; never ignore unrelated
// diagnostics, especially cache or log permission failures.
var runnerCleanupWarning = regexp.MustCompile("^(?:\\[WARNING \\] )?unclosed (?:Runner <salt\\.runner\\.Runner object at 0x[0-9a-f]+>|MasterMinion <salt\\.minion\\.MasterMinion object at 0x[0-9a-f]+>); call ``destroy\\(\\)`` or use as a context manager$")

func onlyRunnerCleanupWarnings(stderr string) bool {
	if strings.TrimSpace(stderr) == "" {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if !runnerCleanupWarning.MatchString(line) {
			return false
		}
	}
	return true
}

func saltDiagnostic(stderr string) error {
	if strings.Contains(stderr, "prep_jid could not store a jid") || strings.Contains(stderr, "Permission denied: '/var/cache/salt/master/jobs/") {
		return errors.New("Salt master job cache is not writable by this account; use an authorized Salt master account (check /var/cache/salt/master/jobs)")
	}
	if strings.Contains(stderr, "Failed to open log file") && strings.Contains(stderr, "/var/log/salt/") {
		return errors.New("Salt log is not writable by this account; use an authorized Salt master account (check /var/log/salt)")
	}
	return errors.New("Salt emitted diagnostics on stderr; check Salt logs")
}

type runner func(context.Context, string, ...string) ([]byte, error)

type Gateway struct {
	ConfigDir string
	run       runner
	slots     chan struct{}
}

func New(configDir string) *Gateway {
	return &Gateway{ConfigDir: configDir, run: execute, slots: make(chan struct{}, 2)}
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > outputLimit-b.Len() {
		return 0, errors.New("Salt output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func execute(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("Salt command timed out or was canceled: %w", ctx.Err())
	}
	if err != nil {
		// Salt stderr may contain private data; do not surface it in the UI.
		if strings.Contains(stderr.String(), "prep_jid could not store a jid") || strings.Contains(stderr.String(), "Failed to open log file") {
			return nil, saltDiagnostic(stderr.String())
		}
		if stderr.Len() > 0 {
			return nil, saltDiagnostic(stderr.String())
		}
		return stdout.Bytes(), fmt.Errorf("Salt command failed: %w (check Salt logs and permissions)", err)
	}
	if stderr.Len() > 0 && !(filepath.Base(binary) == "salt-run" && onlyRunnerCleanupWarnings(stderr.String())) {
		// Diagnostic text must not be interpreted as a successful JSON return.
		return nil, saltDiagnostic(stderr.String())
	}
	return stdout.Bytes(), nil
}

func (g *Gateway) invoke(ctx context.Context, binary string, args ...string) ([]byte, error) {
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if g.ConfigDir != "" {
		args = append([]string{"-c", g.ConfigDir}, args...)
	}
	return g.run(ctx, binary, args...)
}

func decode(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("invalid Salt JSON: %w", err)
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return errors.New("unexpected trailing Salt output")
	}
	return nil
}

func validID(id string) bool {
	return id != "" && !strings.ContainsRune(id, ',') && !strings.HasPrefix(id, "-") &&
		!strings.ContainsFunc(id, unicode.IsControl)
}

func parseIDs(ids []string) ([]string, error) {
	if ids == nil {
		return nil, errors.New("Salt response does not contain an ID list")
	}
	for _, id := range ids {
		if id == "" || strings.ContainsFunc(id, unicode.IsControl) {
			return nil, errors.New("Salt response contains an invalid minion ID")
		}
	}
	return fleet.SortUnique(ids), nil
}

func (g *Gateway) ListAcceptedKeys(ctx context.Context) ([]string, error) {
	data, err := g.invoke(ctx, "salt-key", "--list", "accepted", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("accepted keys: %w", err)
	}
	var response struct {
		Minions []string `json:"minions"`
	}
	if err := decode(data, &response); err != nil {
		return nil, fmt.Errorf("accepted keys: %w", err)
	}
	return parseIDs(response.Minions)
}

func (g *Gateway) ReadPresence(ctx context.Context) ([]string, error) {
	data, err := g.invoke(ctx, "salt-run", "manage.present", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("presence: %w", err)
	}
	var ids []string
	if err := decode(data, &ids); err != nil {
		return nil, fmt.Errorf("presence: %w", err)
	}
	return parseIDs(ids)
}

func (g *Gateway) ReadSelectedGrains(ctx context.Context, id string) (fleet.Grains, error) {
	if !validID(id) {
		return nil, errors.New("minion ID cannot be targeted safely as a single list item")
	}
	data, err := g.invoke(ctx, "salt", "-L", id, "grains.items", "sanitize=True", "--timeout=5", "--static", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("selected grains: %w", err)
	}
	var response map[string]json.RawMessage
	if err := decode(data, &response); err != nil {
		return nil, fmt.Errorf("selected grains: %w", err)
	}
	if len(response) != 1 {
		return nil, errors.New("selected grains: missing or unexpected minion return")
	}
	value, ok := response[id]
	if !ok {
		return nil, errors.New("selected grains: returned minion ID differs from requested ID")
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var fields fleet.Grains
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, errors.New("selected grains: invalid minion return")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, errors.New("selected grains: unexpected trailing minion return")
	}
	return fields, nil
}

type jobInfo struct {
	JID        string          `json:"jid"`
	Function   string          `json:"Function"`
	Target     json.RawMessage `json:"Target"`
	TargetType string          `json:"Target-type"`
	StartTime  string          `json:"StartTime"`
	Minions    []string        `json:"Minions"`
	Result     json.RawMessage `json:"Result"`
	Error      string          `json:"Error"`
}

func summary(jid string, info jobInfo) (jobs.Summary, error) {
	if !jidPattern.MatchString(jid) || info.Function == "" || len(info.Target) == 0 || info.StartTime == "" || info.Error != "" {
		return jobs.Summary{}, errors.New("invalid or expired job metadata")
	}
	var target any
	if err := decode(info.Target, &target); err != nil || target == nil {
		return jobs.Summary{}, errors.New("invalid job target")
	}
	var text string
	switch value := target.(type) {
	case string:
		text = value
	case []any:
		ids := make([]string, len(value))
		for i, id := range value {
			str, ok := id.(string)
			if !ok {
				return jobs.Summary{}, errors.New("invalid job target list")
			}
			ids[i] = str
		}
		text = strings.Join(ids, ", ")
	default:
		return jobs.Summary{}, errors.New("invalid job target type")
	}
	return jobs.Summary{JID: jid, Function: info.Function, Target: text, TargetType: info.TargetType, StartTime: info.StartTime}, nil
}

func (g *Gateway) ListRecentJobs(ctx context.Context) ([]jobs.Summary, error) {
	data, err := g.invoke(ctx, "salt-run", "jobs.list_jobs_filter", "50", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("recent jobs: %w", err)
	}
	var document json.RawMessage
	if err := decode(data, &document); err != nil || len(document) == 0 {
		return nil, errors.New("recent jobs: invalid cache response")
	}
	result := make([]jobs.Summary, 0)
	seen := make(map[string]bool)
	appendJob := func(jid string, value json.RawMessage) error {
		var info jobInfo
		if err := decode(value, &info); err != nil || info.JID != "" && info.JID != jid || seen[jid] {
			return errors.New("invalid, duplicate, or mismatched cache entry")
		}
		item, err := summary(jid, info)
		if err != nil {
			return err
		}
		seen[jid] = true
		result = append(result, item)
		return nil
	}
	switch document[0] {
	case '{':
		var raw map[string]json.RawMessage
		if err := decode(document, &raw); err != nil || raw == nil {
			return nil, errors.New("recent jobs: invalid cache response")
		}
		if len(raw) > 50 {
			return nil, errors.New("recent jobs: cache exceeded bounded listing")
		}
		for jid, value := range raw {
			if err := appendJob(jid, value); err != nil {
				return nil, fmt.Errorf("recent jobs: %w", err)
			}
		}
	case '[':
		var raw []json.RawMessage
		if err := decode(document, &raw); err != nil || raw == nil {
			return nil, errors.New("recent jobs: invalid cache response")
		}
		if len(raw) > 50 {
			return nil, errors.New("recent jobs: cache exceeded bounded listing")
		}
		for _, value := range raw {
			var id struct {
				JID string `json:"JID"`
			}
			if err := decode(value, &id); err != nil || !jidPattern.MatchString(id.JID) {
				return nil, errors.New("recent jobs: invalid JID in cache entry")
			}
			if err := appendJob(id.JID, value); err != nil {
				return nil, fmt.Errorf("recent jobs: %w", err)
			}
		}
	default:
		return nil, errors.New("recent jobs: invalid cache response")
	}
	jobs.Sort(result)
	return result, nil
}

func (g *Gateway) ReadJob(ctx context.Context, jid string) (jobs.Detail, error) {
	if !jidPattern.MatchString(jid) {
		return jobs.Detail{}, errors.New("invalid job ID")
	}
	data, err := g.invoke(ctx, "salt-run", "jobs.list_job", jid, "--out=json", "--no-color")
	if err != nil {
		return jobs.Detail{}, fmt.Errorf("job detail: %w", err)
	}
	var info jobInfo
	if err := decode(data, &info); err != nil || info.JID != jid {
		return jobs.Detail{}, errors.New("job detail: invalid or mismatched job ID")
	}
	item, err := summary(jid, info)
	if err != nil {
		return jobs.Detail{}, fmt.Errorf("job detail: %w", err)
	}
	for _, id := range info.Minions {
		if id == "" {
			return jobs.Detail{}, errors.New("job detail: invalid minion ID")
		}
	}
	if len(info.Result) == 0 {
		return jobs.Detail{}, errors.New("job detail: missing return map")
	}
	var raw map[string]json.RawMessage
	if err := decode(info.Result, &raw); err != nil || raw == nil {
		return jobs.Detail{}, errors.New("job detail: invalid return map")
	}
	result := jobs.Detail{Summary: item, Minions: info.Minions, Returns: make(map[string]jobs.Return, len(raw))}
	for id, value := range raw {
		if id == "" {
			return jobs.Detail{}, errors.New("job detail: invalid returned minion ID")
		}
		var envelope struct {
			Return  json.RawMessage `json:"return"`
			Retcode *int            `json:"retcode"`
			Success *bool           `json:"success"`
		}
		if err := decode(value, &envelope); err != nil || len(envelope.Return) == 0 {
			return jobs.Detail{}, errors.New("job detail: invalid return envelope")
		}
		result.Returns[id] = jobs.Return{Value: envelope.Return, Retcode: envelope.Retcode, Success: envelope.Success}
	}
	return result, nil
}
