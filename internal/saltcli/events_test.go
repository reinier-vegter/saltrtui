package saltcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"saltrtui/internal/events"
)

func TestEventStreamHelper(t *testing.T) {
	if os.Getenv("SALTRTUI_EVENT_HELPER") == "" {
		return
	}
	switch os.Getenv("SALTRTUI_EVENT_HELPER") {
	case "invalid":
		fmt.Fprintln(os.Stdout, "not an event frame")
	case "hold":
		fmt.Fprintln(os.Stdout, "salt/job/1/new\t{\"jid\":\"1\"}")
		time.Sleep(30 * time.Second)
	case "burst":
		for i := 0; i < 100; i++ {
			fmt.Fprintf(os.Stdout, "salt/job/%d/new\t{}\n", i)
		}
	}
	os.Exit(0)
}

func testEventProcess(t *testing.T, mode string) (context.Context, context.CancelFunc, context.CancelFunc, *exec.Cmd) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	processCtx, stop := context.WithCancel(ctx)
	cmd := exec.CommandContext(processCtx, os.Args[0], "-test.run=TestEventStreamHelper")
	cmd.Env = append(os.Environ(), "SALTRTUI_EVENT_HELPER="+mode)
	return ctx, cancel, stop, cmd
}

func awaitEventUpdate(t *testing.T, updates <-chan events.Update) (events.Update, bool) {
	t.Helper()
	select {
	case update, ok := <-updates:
		return update, ok
	case <-time.After(3 * time.Second):
		t.Fatal("event reader did not terminate")
		return events.Update{}, false
	}
}

func TestEventStreamFramingAndChildExit(t *testing.T) {
	ctx, cancel, stop, cmd := testEventProcess(t, "invalid")
	defer cancel()
	updates, err := subscribeEvents(ctx, stop, cmd)
	if err != nil {
		t.Fatal(err)
	}
	update, ok := awaitEventUpdate(t, updates)
	if !ok || update.Err == nil || !strings.Contains(update.Err.Error(), "invalid event tag") {
		t.Fatalf("malformed stream should stop child and report frame error: %+v %t", update, ok)
	}
	if _, ok := awaitEventUpdate(t, updates); ok {
		t.Fatal("event reader left stream open after malformed frame")
	}
}

func TestEventStreamCancellationAndBoundedQueue(t *testing.T) {
	ctx, cancel, stop, cmd := testEventProcess(t, "hold")
	updates, err := subscribeEvents(ctx, stop, cmd)
	if err != nil {
		t.Fatal(err)
	}
	update, ok := awaitEventUpdate(t, updates)
	if !ok || update.Record.JID != "1" {
		t.Fatalf("first streamed event not read: %+v %t", update, ok)
	}
	cancel()
	if _, ok := awaitEventUpdate(t, updates); ok {
		t.Fatal("canceled subscription did not terminate and close")
	}

	ctx, cancel, stop, cmd = testEventProcess(t, "burst")
	defer cancel()
	updates, err = subscribeEvents(ctx, stop, cmd)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately do not read until the child has filled the bounded channel.
	time.Sleep(100 * time.Millisecond)
	count, loss := 0, uint64(0)
	for update := range updates {
		if update.Err == nil {
			count++
		} else {
			loss = update.Dropped
		}
	}
	if count > 32 || count+int(loss) != 100 {
		t.Fatalf("stream queue lost unreported frames: delivered=%d dropped=%d", count, loss)
	}
}
