// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"errors"
	"io"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *machineSuite) TestDirectTCPIPHandler(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{ChannelHandlers: map[string]ssh.ChannelHandler{
		"direct-tcpip": func(_ *ssh.Server, _ *gossh.ServerConn, channel gossh.NewChannel, _ ssh.Context) {
			connection, requests, err := channel.Accept()
			if err != nil {
				return
			}
			defer connection.Close()
			go gossh.DiscardRequests(requests)
			_, _ = connection.Write([]byte("Hello world"))
		},
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{
		LocalPortForwardingCallback: func(ssh.Context, string, uint32) bool { return true },
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": handlers.DirectTCPIPHandler(),
		},
	})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	connection, err := client.Dial("tcp", "localhost:8080")
	c.Assert(err, tc.ErrorIsNil)
	defer connection.Close()

	response, err := io.ReadAll(connection)

	c.Check(err, tc.ErrorIsNil)
	c.Check(response, tc.DeepEquals, []byte("Hello world"))
}

func (s *machineSuite) TestDirectTCPIPHandlerPreservesHalfClose(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{ChannelHandlers: map[string]ssh.ChannelHandler{
		"direct-tcpip": func(_ *ssh.Server, _ *gossh.ServerConn, channel gossh.NewChannel, _ ssh.Context) {
			connection, requests, err := channel.Accept()
			if err != nil {
				return
			}
			defer connection.Close()
			go gossh.DiscardRequests(requests)

			request, err := io.ReadAll(connection)
			if err != nil {
				return
			}
			_, _ = connection.Write(append([]byte("response: "), request...))
		},
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{
		LocalPortForwardingCallback: func(ssh.Context, string, uint32) bool { return true },
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": handlers.DirectTCPIPHandler(),
		},
	})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	connection, err := client.Dial("tcp", "localhost:8080")
	c.Assert(err, tc.ErrorIsNil)
	defer connection.Close()

	_, err = connection.Write([]byte("request"))
	c.Assert(err, tc.ErrorIsNil)
	closeWriter, ok := connection.(interface{ CloseWrite() error })
	c.Assert(ok, tc.IsTrue)
	c.Assert(closeWriter.CloseWrite(), tc.ErrorIsNil)

	response, err := io.ReadAll(connection)
	c.Check(err, tc.ErrorIsNil)
	c.Check(response, tc.DeepEquals, []byte("response: request"))
}

func (s *machineSuite) TestDirectTCPIPHandlerReportsConnectionFailure(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := NewHandlers(destination, connectorFunc(func(context.Context, virtualhostname.Info) (*gossh.Client, error) {
		return nil, errors.New("connection failed")
	}), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{
		LocalPortForwardingCallback: func(ssh.Context, string, uint32) bool { return true },
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": handlers.DirectTCPIPHandler(),
		},
	})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	_, err = client.Dial("tcp", "localhost:8080")

	c.Check(err, tc.ErrorMatches, `ssh: rejected: connect failed \(connecting to machine: connection failed\)`)
}
