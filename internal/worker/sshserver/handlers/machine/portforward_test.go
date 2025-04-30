// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"errors"
	"io"

	ssh "github.com/gliderlabs/ssh"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	virtualhostname "github.com/juju/juju/core/virtualhostname"
)

var sampleServerResponse = []byte("Hello world")

type portForwardSuite struct {
	mockConnector *MockSSHConnector
}

var _ = gc.Suite(&portForwardSuite{})

func (s *portForwardSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockConnector = NewMockSSHConnector(ctrl)

	return ctrl
}

// startTestControllerServer creates a test server that behaves like
// the controller's terminating SSH server with a proxying direct TCPIP
// handler.
func (s *portForwardSuite) startTestControllerServer(_ *gc.C, handler ssh.ChannelHandler) *testServer {
	ts := &testServer{}
	ts.server = &ssh.Server{
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			return true
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": handler,
		},
	}
	ts.listener = bufconn.Listen(1024)
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
	return ts
}

// startTestMachineServer creates a test server that
// emulates the behaviour of an SSH server on the target machine.
func (s *portForwardSuite) startTestMachineServer(_ *gc.C) *testServer {
	ts := &testServer{}
	ts.server = &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": machineHandler,
		},
	}
	ts.listener = bufconn.Listen(1024)
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
	return ts
}

// machineHandler emulates the behaviour of the direct TCPIP handler
// on the target machine. It writes a constant response back to the client.
func machineHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		conn.Close()
		return
	}
	go gossh.DiscardRequests(reqs)

	_, _ = ch.Write(sampleServerResponse)

	ch.Close()
	conn.Close()
}

// TestLocalPortForwarding verifies our proxying direct TCPIP handler,
// validating that a user's request for local port forwarding is
// proxied through the controller's SSH server, sent through to the
// target machine and the response is sent back to the user.
func (s *portForwardSuite) TestLocalPortForwarding(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	handlers := Handlers{
		connector:   s.mockConnector,
		logger:      loggo.GetLogger("test"),
		destination: virtualhostname.Info{},
	}

	controllerServer := s.startTestControllerServer(c, handlers.DirectTCPIPHandler())
	defer controllerServer.listener.Close()

	machineServer := s.startTestMachineServer(c)
	defer machineServer.listener.Close()

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			conn, err := machineServer.listener.Dial()
			if err != nil {
				return nil, err

			}
			sshConn, newChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			})
			if err != nil {
				return nil, err
			}
			return gossh.NewClient(sshConn, newChan, reqs), nil
		},
	)

	conn, err := controllerServer.listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	sshConn, sshChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	c.Assert(err, jc.ErrorIsNil)

	client := gossh.NewClient(sshConn, sshChan, reqs)
	defer client.Close()

	forwardedConn, err := client.Dial("tcp", "localhost:8080")
	c.Assert(err, jc.ErrorIsNil)
	defer forwardedConn.Close()

	result, err := io.ReadAll(forwardedConn)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, sampleServerResponse)
}

func (s *portForwardSuite) TestLocalPortForwardingFailsToConnect(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	handlers := Handlers{
		connector:   s.mockConnector,
		logger:      loggo.GetLogger("test"),
		destination: virtualhostname.Info{},
	}

	controllerServer := s.startTestControllerServer(c, handlers.DirectTCPIPHandler())
	defer controllerServer.listener.Close()

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			return nil, errors.New("my-fake-error")
		},
	)

	conn, err := controllerServer.listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	sshConn, sshChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	c.Assert(err, jc.ErrorIsNil)

	client := gossh.NewClient(sshConn, sshChan, reqs)
	defer client.Close()

	_, err = client.Dial("tcp", "localhost:8080")
	c.Assert(err, gc.ErrorMatches, `ssh: rejected: connect failed \(failed to connect to machine: my-fake-error\)`)
}
