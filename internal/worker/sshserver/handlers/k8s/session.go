// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"

	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
)

// SessionHandler proxies a user SSH session to a Kubernetes container.
func (h *Handlers) SessionHandler(session ssh.Session) {
	handleError := func(err error) {
		h.logger.Errorf(session.Context(), "Kubernetes session proxy failure: %v", err)
		_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
		_ = session.Exit(1)
	}

	namespace, podName, err := h.resolver.ResolveK8sExecInfo(session.Context(), h.destination)
	if err != nil {
		handleError(errors.Annotate(err, "resolving Kubernetes exec information"))
		return
	}
	executor, err := h.getExecutor(namespace)
	if err != nil {
		handleError(errors.Annotate(err, "getting Kubernetes executor"))
		return
	}
	container, _ := h.destination.Container()
	ptyRequest, windowChanges, hasPTY := session.Pty()

	var stdin io.Reader = session
	var stdout, stderr io.Writer = session, session.Stderr()

	// A PTY request needs a real terminal descriptor. The SSH session streams
	// alone do not satisfy terminal detection performed by the executor.
	// Further investigation is needed to understand this fully and whether it
	// can be simplified.

	var proxy *ptyProxy
	if hasPTY {
		proxy, err = newPTYProxy(session, ptyRequest, windowChanges)
		if err != nil {
			handleError(err)
			return
		}
		stdin, stdout, stderr = proxy.Streams()
	}

	command := session.RawCommand()
	if command == "" {
		command = "/bin/sh"
	}
	signals := make(chan ssh.Signal, 1)
	session.Signals(signals)
	// Based on the docstring of `session.Signals`:
	// registering nil will unregister the channel from signal sends.
	defer session.Signals(nil)

	err = executor.Exec(session.Context(), k8sexec.ExecParams{
		PodName:       podName,
		ContainerName: container,
		Commands:      []string{command},
		Stdout:        stdout,
		Stderr:        stderr,
		Stdin:         stdin,
		TTY:           hasPTY,
		Env:           sessionEnvironment(session.Environ(), ptyRequest.Term, hasPTY),
		Signal:        translateSignals(session.Context(), signals),
	}, session.Context().Done())
	if err != nil {
		if exitErr, ok := errors.Cause(err).(k8sexec.ExitError); ok {
			if proxy != nil {
				proxy.Close(exitErr.ExitStatus())
			} else {
				_ = session.Exit(exitErr.ExitStatus())
			}
			return
		}
		annotated := errors.Annotate(err, "executing command in Kubernetes pod")
		if proxy != nil {
			h.logger.Errorf(session.Context(), "Kubernetes session proxy failure: %v", annotated)
			_, _ = stderr.Write([]byte(annotated.Error() + "\n"))
			proxy.Close(1)
		} else {
			handleError(annotated)
		}
		return
	}
	if proxy != nil {
		proxy.Close(0)
	}
}

func sessionEnvironment(env []string, terminal string, hasPTY bool) []string {
	if !hasPTY || terminal == "" {
		return env
	}
	// Verify that the TERM environment variable is set.
	// If it is not, add it to the environment with the value from the PTY request.
	for _, value := range env {
		if len(value) >= len("TERM=") && value[:len("TERM=")] == "TERM=" {
			return env
		}
	}
	return append(env, "TERM="+terminal)
}

func translateSignals(ctx context.Context, signals <-chan ssh.Signal) <-chan syscall.Signal {
	translated := make(chan syscall.Signal, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if value, ok := sshSignals[signal]; ok {
					select {
					case translated <- value:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return translated
}

var sshSignals = map[ssh.Signal]syscall.Signal{
	ssh.SIGABRT: syscall.SIGABRT,
	ssh.SIGALRM: syscall.SIGALRM,
	ssh.SIGFPE:  syscall.SIGFPE,
	ssh.SIGHUP:  syscall.SIGHUP,
	ssh.SIGILL:  syscall.SIGILL,
	ssh.SIGINT:  syscall.SIGINT,
	ssh.SIGKILL: syscall.SIGKILL,
	ssh.SIGPIPE: syscall.SIGPIPE,
	ssh.SIGQUIT: syscall.SIGQUIT,
	ssh.SIGSEGV: syscall.SIGSEGV,
	ssh.SIGTERM: syscall.SIGTERM,
	ssh.SIGUSR1: syscall.SIGUSR1,
	ssh.SIGUSR2: syscall.SIGUSR2,
}

// ptyProxy owns a pseudo-terminal and the goroutines that copy data and resize
// it for an SSH session.
type ptyProxy struct {
	session ssh.Session
	ptmx    *os.File
	tty     *os.File

	cancel     context.CancelFunc
	resizeDone chan struct{}
	outputDone chan struct{}
	wg         sync.WaitGroup
}

const ptyOutputDrainTimeout = 5 * time.Second

func newPTYProxy(session ssh.Session, request ssh.Pty, windowChanges <-chan ssh.Window) (*ptyProxy, error) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, errors.Annotate(err, "opening pseudo-terminal")
	}
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(request.Window.Height),
		Cols: uint16(request.Window.Width),
	}); err != nil {
		_ = tty.Close()
		_ = ptmx.Close()
		return nil, errors.Annotate(err, "setting pseudo-terminal size")
	}

	ctx, cancel := context.WithCancel(session.Context())
	proxy := &ptyProxy{
		session:    session,
		ptmx:       ptmx,
		tty:        tty,
		cancel:     cancel,
		resizeDone: make(chan struct{}),
		outputDone: make(chan struct{}),
	}
	proxy.start(ctx, windowChanges)
	return proxy, nil
}

// Streams returns the terminal streams to pass to the Kubernetes executor.
func (p *ptyProxy) Streams() (io.Reader, io.Writer, io.Writer) {
	return p.tty, p.tty, p.tty
}

// Close drains final command output, closes the SSH session with the
// appropriate status, and waits for all proxy goroutines to stop.
func (p *ptyProxy) Close(status int) {
	// Stop resizing before closing descriptors so Setsize cannot race with
	// File.Close.
	p.cancel()
	<-p.resizeDone

	// Closing the slave signals command completion. Keep the master open until
	// its copier has drained any final command output to the SSH client.
	_ = p.tty.Close()
	select {
	case <-p.outputDone:
	case <-p.session.Context().Done():
	case <-time.After(ptyOutputDrainTimeout):
	}

	_ = p.session.Exit(status)
	_ = p.ptmx.Close()
	p.wg.Wait()
}

func (p *ptyProxy) start(ctx context.Context, windowChanges <-chan ssh.Window) {
	p.wg.Go(func() {
		defer close(p.resizeDone)
		for {
			select {
			case <-ctx.Done():
				return
			case window, ok := <-windowChanges:
				if !ok {
					return
				}
				_ = pty.Setsize(p.ptmx, &pty.Winsize{
					Rows: uint16(window.Height),
					Cols: uint16(window.Width),
				})
			}
		}
	})
	p.wg.Go(func() {
		// Closing the SSH session or master descriptor interrupts this copy.
		_, _ = io.Copy(p.ptmx, p.session)
	})
	p.wg.Go(func() {
		_, _ = io.Copy(p.session, p.ptmx)
		// outputDone lets Close drain output before closing the SSH session.
		close(p.outputDone)
	})
}
