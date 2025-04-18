// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
)

// SSHConnector is an interface that defines the methods required to
// connect to a remote SSH server.
type SSHConnector interface {
	Connect(destination virtualhostname.Info) (*gossh.Client, error)
}

type machineHandlers struct {
	connector SSHConnector
	logger    Logger
}

// newMachineHandlers creates a new machine session handler.
func newMachineHandlers(connector SSHConnector, logger Logger) (*machineHandlers, error) {
	if connector == nil {
		return nil, errors.NotValidf("connector is required")
	}
	if logger == nil {
		return nil, errors.NotValidf("logger is required")
	}
	return &machineHandlers{
		connector: connector,
		logger:    logger,
	}, nil
}

// Handle proxies a user's SSH session to a target unit or machines.
// Connections to machine will be proxied to the machine's SSH server.
// Connections to k8s units will be proxied through the k8s API server.
func (m *machineHandlers) SessionHandler(session ssh.Session, details connectionDetails) error {
	if err := m.machineSessionProxy(session, details.destination); err != nil {
		err = errors.Annotate(err, "failed to proxy machine session")
		return err
	}
	return nil
}

func (s *machineHandlers) machineSessionProxy(userSession ssh.Session, destination virtualhostname.Info) error {
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

func (*machineHandlers) setupShellOrCommand(userSession ssh.Session, machineSSHSession *gossh.Session) error {
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

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32

	OriginAddr string
	OriginPort uint32
}

// MachineDirectTCPIPHandler returns a handler for the DirectTCPIP channel type.
// This handler is used for local port forwarding. While the handler is nearly
// identical to the default DirectTCPIPHandler, it first connects to the target
// machine and proxies the port forwarding request through the machine's SSH server.
func (m *machineHandlers) DirectTCPIPHandler(details connectionDetails) ssh.ChannelHandler {
	return func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		d := localForwardChannelData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, "error parsing forward data: "+err.Error())
			return
		}

		dest := net.JoinHostPort(d.DestAddr, strconv.FormatInt(int64(d.DestPort), 10))

		client, err := m.connector.Connect(details.destination)
		if err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("failed to connect to machine: %s", err.Error()))
			return
		}
		dconn, err := client.DialContext(ctx, "tcp", dest)
		if err != nil {
			_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("failed to dial target: %s", err.Error()))
			return
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			dconn.Close()
			return
		}
		go gossh.DiscardRequests(reqs)

		go func() {
			defer ch.Close()
			defer dconn.Close()
			_, _ = io.Copy(ch, dconn)
		}()
		go func() {
			defer ch.Close()
			defer dconn.Close()
			_, _ = io.Copy(dconn, ch)
		}()
	}
}
