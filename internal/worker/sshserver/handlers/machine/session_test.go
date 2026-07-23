// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bufio"
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

	machine := startSSHTestServer(c, &ssh.Server{Handler: func(session ssh.Session) {
		pty, windowChanges, hasPTY := session.Pty()
		c.Check(hasPTY, tc.IsTrue)
		c.Check(pty.Term, tc.Equals, "xterm")

		_, _ = io.WriteString(session, "shell ready\n")
		var window ssh.Window
		// This loop terminates when we get what we expect
		// or when the server is shutdown and windowChanges closes.
		for {
			window = <-windowChanges
			if window.Height == 30 && window.Width == 100 {
				break
			}
		}
		c.Check(window.Height, tc.Equals, 30)
		c.Check(window.Width, tc.Equals, 100)
		_, _ = io.WriteString(session, "shell done\n")
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

	stdoutReader, stdoutWriter := io.Pipe()
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	session.Stdout = stdoutWriter
	c.Assert(session.RequestPty("xterm", 24, 80, gossh.TerminalModes{}), tc.ErrorIsNil)
	c.Assert(session.Shell(), tc.ErrorIsNil)

	type outputResult struct {
		message string
		err     error
	}
	outputs := make(chan outputResult, 2)
	go func() {
		stdout := bufio.NewReader(stdoutReader)
		message, err := stdout.ReadString('\n')
		outputs <- outputResult{message: message, err: err}
		message, err = stdout.ReadString('\n')
		outputs <- outputResult{message: message, err: err}
	}()

	output := <-outputs
	c.Assert(output.err, tc.ErrorIsNil)
	c.Check(output.message, tc.Equals, "shell ready\r\n")
	c.Assert(session.WindowChange(30, 100), tc.ErrorIsNil)
	c.Assert(session.Wait(), tc.ErrorIsNil)

	output = <-outputs
	c.Assert(output.err, tc.ErrorIsNil)
	c.Check(output.message, tc.Equals, "shell done\r\n")
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
	c.Check(stderr.String(), tc.Equals, "failed to connect to machine: connection failed\n")
}
