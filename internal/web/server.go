package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

const (
	ReadHeaderTimeout = 5 * time.Second
	ReadTimeout       = 15 * time.Second
	WriteTimeout      = 30 * time.Second
	IdleTimeout       = 60 * time.Second
	ShutdownTimeout   = 10 * time.Second
	MaxInFlight       = 32
)

type Options struct {
	Listen        string
	OpenBrowser   bool
	Queries       app.WebQueries
	Operations    app.WebOperations
	Stdout        io.Writer
	Diagnostics   io.Writer
	ListenFunc    func(string, string) (net.Listener, error)
	LaunchBrowser func(context.Context, string) error
	Now           func() time.Time
	RequestID     func() string
}

func Run(ctx context.Context, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := app.ValidateLoopbackListen(opts.Listen); err != nil {
		return fmt.Errorf("validate listen address: %w", err)
	}
	if opts.Queries == nil {
		return errors.New("web queries are required")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Diagnostics == nil {
		opts.Diagnostics = io.Discard
	}
	diagnostics := &lockedWriter{writer: opts.Diagnostics}
	if opts.ListenFunc == nil {
		opts.ListenFunc = net.Listen
	}

	listener, err := opts.ListenFunc("tcp", opts.Listen)
	if err != nil {
		return fmt.Errorf("listen on configured loopback address: %w", err)
	}
	defer listener.Close()
	authority, err := canonicalAuthority(listener.Addr())
	if err != nil {
		return err
	}
	if err := app.ValidateLoopbackListen(authority); err != nil {
		return fmt.Errorf("listener resolved outside loopback policy: %w", err)
	}
	origin := "http://" + authority
	if reconciler, ok := opts.Operations.(app.OperationReconciler); ok {
		if err := reconciler.ReconcileOperations(ctx); err != nil {
			_ = listener.Close()
			return fmt.Errorf("reconcile interrupted operations: %w", err)
		}
	}

	operationRoot, cancelOperations := context.WithCancel(context.Background())
	defer cancelOperations()
	hub := newOperationHub(operationRoot, opts.Operations, opts.Now, opts.RequestID)
	handler, err := NewHandler(HandlerOptions{
		Queries:     opts.Queries,
		Operations:  opts.Operations,
		Authority:   authority,
		Diagnostics: diagnostics,
		Now:         opts.Now,
		RequestID:   opts.RequestID,
		RootContext: operationRoot,
		Hub:         hub,
	})
	if err != nil {
		return fmt.Errorf("initialize web presentation: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	if _, err := fmt.Fprintf(opts.Stdout, "Dashboard: %s/\n", origin); err != nil {
		return fmt.Errorf("write dashboard URL: %w", err)
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	fmt.Fprintf(diagnostics, "event=server_started listen=%s\n", authority)
	if opts.OpenBrowser {
		if opts.LaunchBrowser == nil {
			fmt.Fprintln(diagnostics, "event=browser_launch warning=launcher_unavailable")
		} else if err := opts.LaunchBrowser(ctx, origin+"/"); err != nil {
			fmt.Fprintln(diagnostics, "event=browser_launch warning=launch_failed")
		}
	}

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		started := time.Now()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		cleanupErr := hub.drainAndWait(shutdownCtx)
		cancelOperations()
		err := server.Shutdown(shutdownCtx)
		if cleanupErr != nil {
			return fmt.Errorf("operation cleanup during shutdown: %w", cleanupErr)
		}
		if err != nil {
			_ = server.Close()
		}
		serveResult := <-serveErr
		fmt.Fprintf(diagnostics, "event=server_stopped reason=cancelled duration_ms=%d\n", time.Since(started).Milliseconds())
		if err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if serveResult != nil && !errors.Is(serveResult, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", serveResult)
		}
		return nil
	}
}

type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func canonicalAuthority(addr net.Addr) (string, error) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", fmt.Errorf("resolve listener authority: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("listener is not a numeric loopback address")
	}
	return net.JoinHostPort(ip.String(), port), nil
}
