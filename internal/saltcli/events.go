package saltcli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync/atomic"
	"time"

	"saltrtui/internal/events"
)

// Subscribe occupies its own long-running process, not a short-call slot.
// The caller owns ctx and cancels it on app exit or explicit listener restart.
func (g *Gateway) Subscribe(ctx context.Context) (<-chan events.Update, error) {
	args := []string{"state.event", "node=master", "pretty=False"}
	if g.ConfigDir != "" {
		args = append([]string{"-c", g.ConfigDir}, args...)
	}
	processCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, "salt-run", args...)
	return subscribeEvents(ctx, cancel, cmd)
}

func subscribeEvents(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) (<-chan events.Update, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("event listener stdout: %w", err)
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("event listener start: %w", err)
	}
	updates := make(chan events.Update, 32)
	go func() {
		defer close(updates)
		defer cancel()
		var dropped atomic.Uint64
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), events.MaxFrame+1)
		for scanner.Scan() {
			record, parseErr := events.ParseFrame(scanner.Bytes(), time.Now())
			if parseErr != nil {
				err = parseErr
				break
			}
			select {
			case updates <- events.Update{Record: record, Dropped: dropped.Load()}:
			default:
				dropped.Add(1)
			}
		}
		if err == nil {
			err = scanner.Err()
		}
		// A framing failure must stop the reader's child before Wait; a process
		// that never exits on its own would otherwise hang this goroutine.
		if err != nil {
			cancel()
		}
		waitErr := cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = waitErr
		}
		if err == nil {
			err = errors.New("event listener exited")
		} else {
			err = fmt.Errorf("event listener stopped: %w", err)
		}
		if stderr.Len() > 0 {
			err = errors.New("event listener emitted diagnostics; check Salt logs and permissions")
		}
		select {
		case updates <- events.Update{Err: err, Dropped: dropped.Load()}:
		case <-ctx.Done():
		}
	}()
	return updates, nil
}
