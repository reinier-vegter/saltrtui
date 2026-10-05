// Package highstate describes one minion's highstate returns without Salt CLI types.
package highstate

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrNotSent means the accepted-key check or target validation failed before
// publishing a state job. Any other Apply error must be treated as uncertain.
var ErrNotSent = errors.New("highstate job not sent")

type Step struct {
	ID      string
	Result  *bool // nil means a test-mode proposal.
	Changes json.RawMessage
	Comment string
}

type Report struct {
	Steps                        []Step
	Proposed, Changed, Unchanged int
	Failed                       int
}

type Gateway interface {
	Preview(context.Context, string) (Report, error)
	Apply(context.Context, string) (Report, error)
}
