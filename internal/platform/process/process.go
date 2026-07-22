// Package process provides a small, product-agnostic direct-process boundary.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	DefaultStdoutLimit  = 4 << 20
	DefaultStderrLimit  = 1 << 20
	DefaultCleanupGrace = 5 * time.Second
)

type Request struct {
	Executable   string
	Args         []string
	Dir          string
	Env          []string
	Timeout      time.Duration
	StdoutLimit  int
	StderrLimit  int
	CleanupGrace time.Duration
	Progress     func(Event)
}

type Event struct {
	Stream string
	Data   string
	At     time.Time
}

type Result struct {
	StartedAt, FinishedAt time.Time
	Duration              time.Duration
	ExitCode              int
	Signal                string
	Stdout, Stderr        string
	StdoutTruncated       bool
	StderrTruncated       bool
	DroppedEvents         int
	TimedOut              bool
	Cancelled             bool
	CleanupAttempted      bool
	CleanupComplete       bool
}

type Runner interface {
	Run(context.Context, Request) (Result, error)
}

type DirectRunner struct{}

func (DirectRunner) Run(ctx context.Context, req Request) (Result, error) {
	var result Result
	if ctx == nil {
		return result, fmt.Errorf("process context is required")
	}
	if req.Executable == "" {
		return result, fmt.Errorf("process executable is required")
	}
	if req.Dir == "" {
		return result, fmt.Errorf("process working directory is required")
	}
	if req.Timeout <= 0 {
		return result, fmt.Errorf("process timeout must be positive")
	}
	if req.StdoutLimit <= 0 {
		req.StdoutLimit = DefaultStdoutLimit
	}
	if req.StderrLimit <= 0 {
		req.StderrLimit = DefaultStderrLimit
	}
	if req.CleanupGrace <= 0 {
		req.CleanupGrace = DefaultCleanupGrace
	}

	cmd := exec.Command(req.Executable, req.Args...)
	cmd.Dir = req.Dir
	cmd.Env = append([]string(nil), req.Env...)
	configureOwnedProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("open process stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, fmt.Errorf("open process stderr: %w", err)
	}

	dispatch := newDispatcher(req.Progress)
	outCapture := &limitedCapture{limit: req.StdoutLimit}
	errCapture := &limitedCapture{limit: req.StderrLimit}
	result.StartedAt = time.Now().UTC()
	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("start process: %w", err)
	}
	var drains sync.WaitGroup
	drains.Add(2)
	go func() { defer drains.Done(); copyStream(stdout, outCapture, "stdout", dispatch) }()
	go func() { defer drains.Done(); copyStream(stderr, errCapture, "stderr", dispatch) }()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		result.Cancelled = true
		result.CleanupAttempted = true
		waitErr, result.CleanupComplete = stopAndWait(cmd, waited, req.CleanupGrace)
	case <-timer.C:
		result.TimedOut = true
		result.CleanupAttempted = true
		waitErr, result.CleanupComplete = stopAndWait(cmd, waited, req.CleanupGrace)
	}
	drains.Wait()
	result.DroppedEvents = dispatch.close()
	result.FinishedAt = time.Now().UTC()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	result.Stdout, result.Stderr = outCapture.String(), errCapture.String()
	result.StdoutTruncated, result.StderrTruncated = outCapture.truncated, errCapture.truncated
	result.ExitCode = -1
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
		result.Signal = processSignal(cmd.ProcessState)
	}
	if !result.CleanupAttempted {
		result.CleanupComplete = true
	}
	if result.TimedOut {
		return result, fmt.Errorf("process timed out after %s", req.Timeout)
	}
	if result.Cancelled {
		return result, context.Canceled
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return result, fmt.Errorf("process exited with code %d", result.ExitCode)
		}
		return result, fmt.Errorf("wait for process: %w", waitErr)
	}
	return result, nil
}

type limitedCapture struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *limitedCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		w.truncated = true
		return written, nil
	}
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
		return written, nil
	}
	_, _ = w.buf.Write(p)
	return written, nil
}

func (w *limitedCapture) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func copyStream(src io.Reader, dst io.Writer, stream string, d *dispatcher) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = dst.Write(buf[:n])
			d.emit(Event{Stream: stream, Data: string(buf[:n]), At: time.Now().UTC()})
		}
		if err != nil {
			return
		}
	}
}

type dispatcher struct {
	sink    func(Event)
	ch      chan Event
	done    chan struct{}
	mu      sync.Mutex
	dropped int
}

func newDispatcher(sink func(Event)) *dispatcher {
	d := &dispatcher{sink: sink}
	if sink != nil {
		d.ch, d.done = make(chan Event, 128), make(chan struct{})
		go func() {
			defer close(d.done)
			for event := range d.ch {
				sink(event)
			}
		}()
	}
	return d
}

func (d *dispatcher) emit(event Event) {
	if d.sink == nil {
		return
	}
	select {
	case d.ch <- event:
	default:
		d.mu.Lock()
		d.dropped++
		d.mu.Unlock()
	}
}

func (d *dispatcher) close() int {
	if d.sink != nil {
		close(d.ch)
		<-d.done
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dropped
}
