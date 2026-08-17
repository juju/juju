// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *machineSuite) TestSessionHandlerProxiesCommand(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{Handler: func(session ssh.Session) {
		c.Check(session.RawCommand(), tc.Equals, "echo hello")
		_, _ = io.WriteString(session, "hello\n")
		_, _ = io.WriteString(session.Stderr(), "warning\n")
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run("echo hello")

	c.Check(err, tc.ErrorIsNil)
	c.Check(stdout.String(), tc.Equals, "hello\n")
	c.Check(stderr.String(), tc.Equals, "warning\n")
}

func (s *machineSuite) TestSessionHandlerPropagatesCommandExitCode(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{Handler: func(session ssh.Session) {
		c.Check(session.RawCommand(), tc.Equals, "exit 3")
		_ = session.Exit(3)
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	err = session.Run("exit 3")
	var exitErr *gossh.ExitError
	c.Assert(errors.As(err, &exitErr), tc.IsTrue)
	c.Check(exitErr.ExitStatus(), tc.Equals, 3)
}

func (s *machineSuite) TestSessionHandlerProxiesPTYAndWindowChanges(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	ready := make(chan struct{})

	machine := startSSHTestServer(c, &ssh.Server{
		PtyCallback: func(_ ssh.Context, pty ssh.Pty) bool {
			c.Check(pty.Term, tc.Equals, "xterm")
			c.Check(pty.Window.Height, tc.Equals, 24)
			c.Check(pty.Window.Width, tc.Equals, 80)
			return true
		},
		Handler: func(session ssh.Session) {
			close(ready)
			_, _ = io.WriteString(session, "shell done\n")
		},
	})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	err = session.RequestPty("xterm", 24, 80, gossh.TerminalModes{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(session.Shell(), tc.ErrorIsNil)

	select {
	case <-ready:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for shell to start")
	}
	c.Assert(session.WindowChange(30, 100), tc.ErrorIsNil)
	c.Assert(session.Wait(), tc.ErrorIsNil)
	c.Check(stdout.String(), tc.Equals, "shell done\r\n")
}

func (s *machineSuite) TestSessionHandlerReportsConnectionFailure(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	handlers, err := NewHandlers(destination, connectorFunc(func(context.Context, virtualhostname.Info) (*gossh.Client, error) {
		return nil, errors.New("connection failed")
	}), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{Handler: handlers.SessionHandler})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr

	err = session.Run("echo hello")

	c.Check(err, tc.ErrorMatches, "Process exited with status 1")
	c.Check(stderr.String(), tc.Equals, "failed to connect to machine: connection failed\r\n")
}
