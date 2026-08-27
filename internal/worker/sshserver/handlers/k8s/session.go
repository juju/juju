// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"context"
	"io"
	"strings"
	"syscall"

	"github.com/juju/errors"
	ssh "github.com/tailscale/gliderssh"

	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
)

// SessionChannelHandler adapts the raw session channel to Gliderlabs' session
// handler. Kubernetes execution needs the parsed session values, unlike the
// machine proxy which forwards the channel requests unchanged.
func (h *Handlers) SessionChannelHandler() ssh.ChannelHandler {
	return ssh.DefaultSessionHandler
}

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

	cloudSpec, err := h.resolver.CloudSpecForSSH(session.Context(), h.destination)
	if err != nil {
		handleError(errors.Annotate(err, "getting Kubernetes cloud spec"))
		return
	}

	executor, err := h.getExecutor(namespace, cloudSpec)
	if err != nil {
		handleError(errors.Annotate(err, "getting Kubernetes executor"))
		return
	}

	container, ok := h.destination.Container()
	if !ok {
		handleError(errors.New("destination is not a container target"))
		return
	}

	ptyRequest, windowChanges, hasPTY := session.Pty()
	var terminalSizeQueue k8sexec.TerminalSizeQueue
	if hasPTY {
		terminalSizeQueue = newTerminalSizeQueue(session.Context(), ptyRequest.Window, windowChanges)
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

	h.metrics.ObserveTimeToSession(session.Context())
	err = executor.Exec(session.Context(), k8sexec.ExecParams{
		PodName:           podName,
		ContainerName:     container,
		Commands:          []string{command},
		Stdout:            session,
		Stderr:            sessionStderr(session, hasPTY),
		Stdin:             session,
		TTY:               hasPTY,
		TerminalSizeQueue: terminalSizeQueue,
		Env:               sessionEnvironment(session.Environ(), ptyRequest.Term, hasPTY),
		Signal:            translateSignals(session.Context(), signals),
	}, session.Context().Done())
	if err != nil {
		if exitErr, ok := errors.AsType[k8sexec.ExitError](err); ok {
			_ = session.Exit(exitErr.ExitStatus())
			return
		}
		handleError(errors.Annotate(err, "executing command in Kubernetes pod"))
		return
	}
	_ = session.Exit(0)
}

func sessionStderr(session ssh.Session, hasPTY bool) io.Writer {
	if hasPTY {
		return nil
	}
	return session.Stderr()
}

func sessionEnvironment(env []string, terminal string, hasPTY bool) []string {
	if !hasPTY || terminal == "" {
		return env
	}
	// Verify that the TERM environment variable is set.
	// If it is not, add it to the environment with the value from the PTY request.
	for _, value := range env {
		if strings.HasPrefix(value, "TERM=") {
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

type terminalSizeQueue struct {
	ctx           context.Context
	initial       k8sexec.TerminalSize
	windowChanges <-chan ssh.Window
	getInitial    bool
}

func newTerminalSizeQueue(ctx context.Context, initial ssh.Window, windowChanges <-chan ssh.Window) *terminalSizeQueue {
	return &terminalSizeQueue{
		ctx: ctx,
		initial: k8sexec.TerminalSize{
			Width:  uint16(initial.Width),
			Height: uint16(initial.Height),
		},
		windowChanges: windowChanges,
	}
}

func (q *terminalSizeQueue) Next() *k8sexec.TerminalSize {
	// Get initial value once
	if !q.getInitial {
		q.getInitial = true
		return &k8sexec.TerminalSize{
			Width:  q.initial.Width,
			Height: q.initial.Height,
		}
	}

	select {
	case <-q.ctx.Done():
		return nil
	case window, ok := <-q.windowChanges:
		if !ok {
			return nil
		}
		return &k8sexec.TerminalSize{
			Width:  uint16(window.Width),
			Height: uint16(window.Height),
		}
	}
}
