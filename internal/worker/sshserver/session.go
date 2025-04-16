// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/state"
)

type stubSessionHandler struct{}

// Handle is a stub implementation of the SessionHandler interface.
// It currently does nothing but will be replaced with a real implementation
// that proxies user's requests to the target unit or machine.
func (s *stubSessionHandler) Handle(session ssh.Session, destination virtualhostname.Info) {
}

// SSHConnector is an interface that defines the methods required to
// connect to a remote SSH server.
type SSHConnector interface {
	Connect(destination virtualhostname.Info) (*gossh.Client, error)
}

type sessionHandler struct {
	connector SSHConnector
	modelType state.ModelType
	logger    Logger
}

// Handle proxies a user's SSH session to a target unit or machines.
// Connections to machine will be proxied to the machine's SSH server.
// Connections to k8s units will be proxied through the k8s API server.
func (s *sessionHandler) Handle(session ssh.Session, destination virtualhostname.Info) {
	handleError := func(err error) {
		s.logger.Errorf("proxy failure: %v", err)
		_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
		_ = session.Exit(1)
	}

	switch s.modelType {
	case state.ModelTypeCAAS:
		if err := s.k8sSessionProxy(session); err != nil {
			err = errors.Annotate(err, "failed to proxy k8s session")
			handleError(err)
		}
	case state.ModelTypeIAAS:
		if err := s.machineSessionProxy(session, destination); err != nil {
			err = errors.Annotate(err, "failed to proxy machine session")
			handleError(err)
		}
	default:
		handleError(errors.Errorf("unknown model type %s", s.modelType))
	}
}

func (s *sessionHandler) k8sSessionProxy(_ ssh.Session) error {
	return errors.New("k8s session proxy not implemented")
}

func (s *sessionHandler) machineSessionProxy(userSession ssh.Session, destination virtualhostname.Info) error {
	client, err := s.connector.Connect(destination)
	if err != nil {
		return err
	}
	defer client.Close()

	machineSSHSession, err := client.NewSession()
	if err != nil {
		return err
	}
	defer machineSSHSession.Close()

	machineSSHSession.Stdin = userSession
	machineSSHSession.Stdout = userSession
	machineSSHSession.Stderr = userSession.Stderr()

	err = s.setupShellOrCommand(userSession, machineSSHSession)
	if err != nil {
		return err
	}

	return machineSSHSession.Wait()
}

func (*sessionHandler) setupShellOrCommand(userSession ssh.Session, machineSSHSession *gossh.Session) error {
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
