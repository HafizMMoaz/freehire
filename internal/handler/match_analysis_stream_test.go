package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// stalledConn models the socket of a reader that stopped reading: writes block until the
// write deadline the caller set, then fail the way a real one does. With no deadline set
// it blocks for far longer than any test would wait — which is precisely the production
// hazard, since SetWriteDeadline(time.Time{}) clears the deadline for good.
type stalledConn struct {
	net.Conn
	mu        sync.Mutex
	deadline  time.Time
	deadlines int
}

func (c *stalledConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	c.deadlines++
	return nil
}

func (c *stalledConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	d := c.deadline
	c.mu.Unlock()
	if d.IsZero() {
		time.Sleep(time.Minute) // unbounded in production; the test must never reach this
		return len(p), nil
	}
	if wait := time.Until(d); wait > 0 {
		time.Sleep(wait)
	}
	return 0, os.ErrDeadlineExceeded
}

func (c *stalledConn) deadlinesSet() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadlines
}

// The stream clears the connection's write deadline so the server's 10s WriteTimeout can't
// kill a long analysis — but an unbounded write lets a reader that went away block Flush
// forever, stranding the analysis goroutine holding the lock. Every write must therefore
// carry its own deadline, refreshed per write so a slow-but-alive reader is never cut off.
func TestSSEStream_BoundsEveryWriteSoADeadReaderCannotStrandTheStream(t *testing.T) {
	conn := &stalledConn{}
	s := newSSEStream(bufio.NewWriter(conn), conn, 50*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.event("meta", map[string]bool{"has_cv": true})
		s.event("stage_start", map[string]int{"stage": 1})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writes did not return; a dead reader can still strand the stream")
	}
	if got := conn.deadlinesSet(); got < 2 {
		t.Errorf("write deadlines set = %d, want one per write (>=2) so a slow reader is not cut off", got)
	}
}

// Without a connection (the handler captures a nil conn) the stream must still write —
// the deadline is simply not settable, exactly as before.
func TestSSEStream_NilConnStillWrites(t *testing.T) {
	var buf bytes.Buffer
	s := newSSEStream(bufio.NewWriter(&buf), nil, time.Second)

	s.event("meta", map[string]bool{"has_cv": false})
	s.comment("keepalive")

	if !strings.Contains(buf.String(), "event: meta") {
		t.Errorf("body = %q, want the meta event written", buf.String())
	}
}

// streamFaultHub builds an isolated hub over a recording transport. It deliberately
// avoids the global sentry.Init that sentryApp uses: these tests assert what one hub
// delivered, and a package-global client would couple them to test execution order.
func streamFaultHub(t *testing.T) (*sentry.Hub, *recordingTransport) {
	t.Helper()
	tr := &recordingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@o0.ingest.sentry.io/0",
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("sentry.NewClient: %v", err)
	}
	return sentry.NewHub(client, sentry.NewScope()), tr
}

// The whole point of the seam: a fault raised after the body began streaming is never
// returned to Fiber, so RenderError never sees it. Without this call such a failure is
// invisible in Sentry — which is exactly how the fit-stream outage went unreported.
func TestReportStreamFault_ReportsUnexpectedFault(t *testing.T) {
	hub, tr := streamFaultHub(t)

	reportStreamFault(hub, errString("llm chain exploded"))

	if got := tr.count(); got != 1 {
		t.Errorf("sentry events = %d, want exactly 1", got)
	}
}

// A reader that walked away is not an application fault. classify() already encodes
// that rule for the returned-error path; the streaming path must not disagree with it,
// or every closed tab becomes an error-inbox entry.
func TestReportStreamFault_IgnoresClientDisconnect(t *testing.T) {
	hub, tr := streamFaultHub(t)

	reportStreamFault(hub, fmt.Errorf("stage 1: %w", context.Canceled))

	if got := tr.count(); got != 0 {
		t.Errorf("sentry events = %d, want 0 for a client disconnect", got)
	}
}

// Sentry is opt-in and env-gated: with no DSN the middleware installs no hub, so the
// writer holds a nil one. Reporting must degrade to nothing rather than panic inside
// the SSE goroutine, where a panic would take the whole stream down.
func TestReportStreamFault_NilHubIsNoop(t *testing.T) {
	reportStreamFault(nil, errString("llm chain exploded"))
}
