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
// The session is the user's SSH session, and createRemote creates a an
// SSH session to the target machine.
func (h *Handlers) SessionHandler(session ssh.Session) {
	handleProxy(h, session.Context(), proxyConfig[*gossh.Session]{
		createRemote: func(_ context.Context, client *gossh.Client) (*gossh.Session, error) {
			machineSession, err := client.NewSession()
			if err != nil {
				return nil, err
			}

			machineSession.Stdin = session
			machineSession.Stdout = session
			machineSession.Stderr = session.Stderr()
			if err := setupShellOrCommand(session, machineSession); err != nil {
				_ = machineSession.Close()
				return nil, err
			}
			return machineSession, nil
		},
		run: func(remote *gossh.Session) error {
			err := remote.Wait()
			if err == nil {
				return nil
			}
			return errors.Annotate(err, "waiting for SSH session to machine")
		},
		onError: func(err error) { h.handleError(session, err) },
	})
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
				// Forward window size changes for responsive terminal behaviour.
				_ = machineSession.WindowChange(window.Height, window.Width)
			}
		}
	}()
	return nil
}
