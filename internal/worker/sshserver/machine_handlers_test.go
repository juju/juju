// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"bytes"
	"io"
	net "net"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
)

var sampleServerResponse = []byte("Hello world")

type machineHandlersSuite struct {
	userSession   *userSession
	mockConnector *MockSSHConnector
}

var _ = gc.Suite(&machineHandlersSuite{})

type testServer struct {
	server   *ssh.Server
	serverRx []byte
	listener *bufconn.Listener
}

// startTestServer creates a test server that emulates the
// SSH server of the target machine.
// Defer the returned listener's Close() method to cleanup the server.
func startTestServer(_ *gc.C) *testServer {
	ts := &testServer{}
	ts.server = &ssh.Server{
		Handler: func(session ssh.Session) {
			_, _, isPty := session.Pty()
			if isPty {
				ts.serverRx, _ = io.ReadAll(session)
				_, _ = io.WriteString(session, "Hello from the server!\n")
				_, _ = io.WriteString(session.Stderr(), "An error from the server!\n")
			} else {
				ts.serverRx = []byte(session.RawCommand())
				_, _ = io.WriteString(session, "No PTY requested.\n")
				_, _ = io.WriteString(session.Stderr(), "An error from the server!\n")
			}
		},
	}
	ts.listener = bufconn.Listen(1024)
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
	return ts
}

type userSession struct {
	ssh.Session
	stdin         bytes.Buffer
	stdout        bytes.Buffer
	stderr        bytes.Buffer
	isPty         bool
	clientCommand string
	exitCode      int
}

func (u *userSession) Write(p []byte) (n int, err error) {
	return u.stdout.Write(p)
}

func (u *userSession) Read(p []byte) (n int, err error) {
	return u.stdin.Read(p)
}

func (u *userSession) Stderr() io.ReadWriter {
	return &u.stderr
}

func (u *userSession) Pty() (ssh.Pty, <-chan ssh.Window, bool) {
	return ssh.Pty{}, nil, u.isPty
}

func (u *userSession) RawCommand() string {
	return u.clientCommand
}

func (u *userSession) Exit(code int) error {
	u.exitCode = code
	return nil
}

func (s *machineHandlersSuite) setupUserSession(_ *gc.C, withPty bool, clientMessage string) {
	s.userSession = &userSession{
		isPty: withPty,
	}
	if withPty {
		s.userSession.stdin.Write([]byte(clientMessage))
	} else {
		s.userSession.clientCommand = clientMessage
	}
}

func (s *machineHandlersSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockConnector = NewMockSSHConnector(ctrl)
	return ctrl
}

type closeChecker struct {
	net.Conn
	closed atomic.Bool
}

func (c *closeChecker) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func (s *machineHandlersSuite) TestMachineSessionProxy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isPty := true
	s.setupUserSession(c, isPty, "Hello from the client!\n")

	testServer := startTestServer(c)
	defer testServer.listener.Close()

	machineConn, err := testServer.listener.Dial()
	c.Assert(err, jc.ErrorIsNil)
	machineConn = &closeChecker{Conn: machineConn}
	defer machineConn.Close()

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			sshConn, newChan, reqs, err := gossh.NewClientConn(machineConn, "", &gossh.ClientConfig{
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			})
			if err != nil {
				return nil, err
			}
			return gossh.NewClient(sshConn, newChan, reqs), nil
		},
	)

	sessionHandler, err := newMachineHandlers(s.mockConnector, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)
	err = sessionHandler.machineSessionProxy(s.userSession, virtualhostname.Info{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.userSession.stdout.String(), gc.Equals, "Hello from the server!\r\n")
	c.Check(s.userSession.stderr.String(), gc.Equals, "An error from the server!\n")
	c.Check(string(testServer.serverRx), gc.Equals, "Hello from the client!\n")

	// Check the connection to the machine is closed.
	closeCheck, _ := machineConn.(*closeChecker)
	c.Assert(closeCheck, gc.NotNil)
	c.Check(closeCheck.closed.Load(), gc.Equals, true)
}

func (s *machineHandlersSuite) TestMachineSessionHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isPty := false
	s.setupUserSession(c, isPty, "neovim")

	testServer := startTestServer(c)
	defer testServer.listener.Close()

	conn, err := testServer.listener.Dial()
	c.Assert(err, jc.ErrorIsNil)

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			sshConn, newChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			})
			if err != nil {
				return nil, err
			}
			return gossh.NewClient(sshConn, newChan, reqs), nil
		},
	)

	machineHandlers, err := newMachineHandlers(s.mockConnector, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	err = machineHandlers.machineSessionProxy(s.userSession, virtualhostname.Info{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.userSession.stdout.String(), gc.Equals, "No PTY requested.\n")
	c.Check(s.userSession.stderr.String(), gc.Equals, "An error from the server!\n")
	c.Check(string(testServer.serverRx), gc.Equals, "neovim")
}

func (s *machineHandlersSuite) TestConnectToMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isPty := false
	s.setupUserSession(c, isPty, "neovim")

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			return nil, errors.New("fake-connection-error")
		},
	)

	machineHandlers, err := newMachineHandlers(s.mockConnector, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	err = machineHandlers.SessionHandler(s.userSession, connectionDetails{})
	c.Check(err, gc.ErrorMatches, "failed to proxy machine session: fake-connection-error")
}

// startTestPortforwardServer creates a test server that behaves like
// the controller's terminating SSH server with a proxying direct TCPIP
// handler.
func startTestControllerPortforwardServer(_ *gc.C, handler ssh.ChannelHandler) *testServer {
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

// startTestMachinePortforwardServer creates a test server that
// emulates the behaviour of an SSH server on the target machine.
func startTestMachinePortforwardServer(_ *gc.C) *testServer {
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
func (s *machineHandlersSuite) TestLocalPortForwarding(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineHandlers, err := newMachineHandlers(s.mockConnector, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	details := connectionDetails{
		destination: virtualhostname.Info{},
	}

	controllerServer := startTestControllerPortforwardServer(c, machineHandlers.DirectTCPIPHandler(details))
	defer controllerServer.listener.Close()

	machineServer := startTestMachinePortforwardServer(c)
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

func (s *machineHandlersSuite) TestLocalPortForwardingFailsToConnect(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineHandlers, err := newMachineHandlers(s.mockConnector, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	details := connectionDetails{
		destination: virtualhostname.Info{},
	}

	controllerServer := startTestControllerPortforwardServer(c, machineHandlers.DirectTCPIPHandler(details))
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
