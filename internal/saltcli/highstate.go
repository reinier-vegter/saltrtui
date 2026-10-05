package saltcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"saltrtui/internal/highstate"
)

const maxHighstateSteps = 2000

func safeHighstateID(id string) bool {
	return len(id) <= 255 && validID(id) && !strings.ContainsAny(id, "*?[]\\/") && !strings.ContainsFunc(id, unicode.IsSpace)
}

func (g *Gateway) acceptedHighstateID(ctx context.Context, id string) error {
	if !safeHighstateID(id) {
		return fmt.Errorf("%w: minion ID is unsafe for exact list targeting", highstate.ErrNotSent)
	}
	ids, err := g.ListAcceptedKeys(ctx)
	if err != nil {
		return fmt.Errorf("%w: fresh accepted-key check failed: %v", highstate.ErrNotSent, err)
	}
	for _, accepted := range ids {
		if accepted == id {
			return nil
		}
	}
	return fmt.Errorf("%w: selected minion is no longer accepted", highstate.ErrNotSent)
}

func parseHighstate(data []byte, id string, preview bool) (highstate.Report, error) {
	var envelope map[string]json.RawMessage
	if err := decode(data, &envelope); err != nil || len(envelope) != 1 {
		return highstate.Report{}, errors.New("highstate: missing or unexpected minion return")
	}
	value, ok := envelope[id]
	if !ok {
		return highstate.Report{}, errors.New("highstate: returned minion differs from requested ID")
	}
	var raw map[string]json.RawMessage
	if err := decode(value, &raw); err != nil || len(raw) == 0 || len(raw) > maxHighstateSteps {
		return highstate.Report{}, errors.New("highstate: missing or invalid state results")
	}
	ids := make([]string, 0, len(raw))
	for stepID := range raw {
		if stepID == "" || strings.ContainsFunc(stepID, unicode.IsControl) {
			return highstate.Report{}, errors.New("highstate: invalid state ID")
		}
		ids = append(ids, stepID)
	}
	sort.Strings(ids)
	report := highstate.Report{Steps: make([]highstate.Step, 0, len(ids))}
	for _, stepID := range ids {
		var fields struct {
			Result  json.RawMessage `json:"result"`
			Changes json.RawMessage `json:"changes"`
			Comment *string         `json:"comment"`
		}
		if err := decode(raw[stepID], &fields); err != nil || fields.Result == nil || fields.Comment == nil || len(fields.Changes) == 0 {
			return highstate.Report{}, errors.New("highstate: malformed state result")
		}
		var result *bool
		switch string(fields.Result) {
		case "true", "false":
			var state bool
			_ = json.Unmarshal(fields.Result, &state)
			result = &state
		case "null":
			if !preview {
				return highstate.Report{}, errors.New("highstate: apply returned test-only state; outcome not verified")
			}
		default:
			return highstate.Report{}, errors.New("highstate: invalid state status")
		}
		var changes map[string]json.RawMessage
		if err := decode(fields.Changes, &changes); err != nil || changes == nil {
			return highstate.Report{}, errors.New("highstate: invalid state changes")
		}
		step := highstate.Step{ID: stepID, Result: result, Changes: bytes.Clone(fields.Changes), Comment: *fields.Comment}
		report.Steps = append(report.Steps, step)
		switch {
		case result == nil:
			report.Proposed++
		case !*result:
			report.Failed++
		case len(changes) > 0:
			report.Changed++
		default:
			report.Unchanged++
		}
	}
	return report, nil
}

func (g *Gateway) highstate(ctx context.Context, id string, preview bool) (highstate.Report, error) {
	if err := g.acceptedHighstateID(ctx, id); err != nil {
		return highstate.Report{}, err
	}
	args := []string{"-L", id, "state.highstate"}
	if preview {
		args = append(args, "test=True")
	} else {
		args = append(args, "test=False")
	}
	args = append(args, "--timeout=180", "--static", "--out=json", "--no-color")
	data, err := g.invoke(ctx, "salt", args...)
	if err != nil {
		// Salt can return a nonzero exit status for failed states while still
		// providing an exact, complete per-minion JSON result. Show those
		// failures, but never reinterpret a nonzero exit with all-success
		// states (or an incomplete response) as success.
		if len(data) > 0 {
			if report, parseErr := parseHighstate(data, id, preview); parseErr == nil && report.Failed > 0 {
				return report, nil
			}
		}
		return highstate.Report{}, fmt.Errorf("highstate outcome unknown; do not retry automatically: %w", err)
	}
	return parseHighstate(data, id, preview)
}

func (g *Gateway) Preview(ctx context.Context, id string) (highstate.Report, error) {
	return g.highstate(ctx, id, true)
}

func (g *Gateway) Apply(ctx context.Context, id string) (highstate.Report, error) {
	return g.highstate(ctx, id, false)
}

var _ highstate.Gateway = (*Gateway)(nil)
