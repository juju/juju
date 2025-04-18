// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

// SessionHandler proxies a user's SSH session to a machine,
// handling both PTY sessions and single commands.
func (s *Handlers) SessionHandler(userSession ssh.Session) {
	handleError := func(err error) {
		s.logger.Errorf("proxy failure: %v", err)
		_, _ = userSession.Stderr().Write([]byte(err.Error() + "\n"))
		_ = userSession.Exit(1)
	}

	client, err := s.connector.Connect(s.destination)
	if err != nil {
		handleError(errors.Annotate(err, "failed to connect to machine"))
		return
	}
	defer client.Close()

	machineSSHSession, err := client.NewSession()
	if err != nil {
		handleError(errors.Annotate(err, "failed to create SSH session to machine"))
		return
	}
	defer machineSSHSession.Close()

	machineSSHSession.Stdin = userSession
	machineSSHSession.Stdout = userSession
	machineSSHSession.Stderr = userSession.Stderr()

	err = s.setupShellOrCommand(userSession, machineSSHSession)
	if err != nil {
		handleError(err)
		return
	}

	if err := machineSSHSession.Wait(); err != nil {
		handleError(errors.Annotate(err, "failed to wait for SSH session to machine"))
		return
	}
}

func (*Handlers) setupShellOrCommand(userSession ssh.Session, machineSSHSession *gossh.Session) error {
	pty, windowChan, isPty := userSession.Pty()
	if isPty {
		// The Gliderlabs SSH server doesn't properly handle terminal modes.
		// See https://github.com/gliderlabs/ssh/issues/98
		// and https://github.com/gliderlabs/ssh/pull/210
		// This will impact terminal behaviour for interactive sessions but can be
		// addressed by using the above patches in a fork of the Gliderlabs SSH server.
		if err := machineSSHSession.RequestPty(pty.Term, pty.Window.Height, pty.Window.Width, gossh.TerminalModes{
			gossh.ECHO: 1,
		}); err != nil {
			return err
		}
		if err := machineSSHSession.Shell(); err != nil {
			return err
		}

		// Handle window size changes
		go func() {
			for w := range windowChan {
				_ = machineSSHSession.WindowChange(w.Height, w.Width)
			}
		}()
	} else {
		return machineSSHSession.Start(userSession.RawCommand())
	}
	return nil
}
