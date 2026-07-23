// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

// SessionHandler proxies a shell or command session to the target machine.
func (h *Handlers) SessionHandler(session ssh.Session) {
	handleError := func(err error) {
		h.logger.Errorf(session.Context(), "machine session proxy failure: %v", err)
		_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))

		var exitErr *gossh.ExitError
		if errors.As(err, &exitErr) {
			_ = session.Exit(exitErr.ExitStatus())
			return
		}
		_ = session.Exit(1)
	}

	client, err := h.connector.Connect(session.Context(), h.destination)
	if err != nil {
		handleError(errors.Annotate(err, "connecting to machine"))
		return
	}
	defer client.Close()
	stop := context.AfterFunc(session.Context(), func() {
		_ = client.Close()
	})
	defer stop()

	machineSession, err := client.NewSession()
	if err != nil {
		handleError(errors.Annotate(err, "creating SSH session to machine"))
		return
	}
	defer machineSession.Close()

	machineSession.Stdin = session
	machineSession.Stdout = session
	machineSession.Stderr = session.Stderr()

	if err := setupShellOrCommand(session, machineSession); err != nil {
		handleError(err)
		return
	}
	if err := machineSession.Wait(); err != nil {
		handleError(errors.Annotate(err, "waiting for SSH session to machine"))
	}
}

func setupShellOrCommand(userSession ssh.Session, machineSession *gossh.Session) error {
	pty, windowChanges, hasPTY := userSession.Pty()
	if !hasPTY {
		return machineSession.Start(userSession.RawCommand())
	}

	// The Gliderlabs SSH server does not currently forward terminal modes.
	// See https://github.com/gliderlabs/ssh/issues/98 and
	// https://github.com/gliderlabs/ssh/pull/210.
	// This will impact terminal behaviour for interactive sessions but can be
	// addressed by using the above patches in a fork of the Gliderlabs SSH server.
	if err := machineSession.RequestPty(pty.Term, pty.Window.Height, pty.Window.Width, gossh.TerminalModes{
		gossh.ECHO: 1,
	}); err != nil {
		return err
	}
	if err := machineSession.Shell(); err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-userSession.Context().Done():
				return
			case window, ok := <-windowChanges:
				if !ok {
					return
				}
				_ = machineSession.WindowChange(window.Height, window.Width)
			}
		}
	}()
	return nil
}
