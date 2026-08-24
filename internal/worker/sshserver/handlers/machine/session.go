// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/juju/errors"
	ssh "github.com/tailscale/gliderssh"
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

	if err := machineSession.RequestPty(pty.Term, pty.Window.Height, pty.Window.Width, pty.Modes); err != nil {
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

	if command := userSession.RawCommand(); command != "" {
		return machineSession.Start(command)
	}
	return machineSession.Shell()
}
