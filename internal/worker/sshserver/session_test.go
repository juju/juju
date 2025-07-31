// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"bytes"
	"io"
	net "net"
	"sync/atomic"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type machineSessionSuite struct {
	userSession   *userSession
	mockConnector *MockSSHConnector
}

func TestMachineSessionSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &machineSessionSuite{})
}

type testServer struct {
	server   *ssh.Server
	serverRx []byte
	listener *bufconn.Listener
}

// startTestServer creates a test server that emulates the
// SSH server of the target machine.
// Defer the returned listener's Close() method to cleanup the server.
func startTestServer(_ *tc.C) *testServer {
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
	windowChanges := make(chan ssh.Window)
	// close immediately to avoid a leaked go routine.
	close(windowChanges)
	return ssh.Pty{}, windowChanges, u.isPty
}

func (u *userSession) RawCommand() string {
	return u.clientCommand
}

func (u *userSession) Exit(code int) error {
	u.exitCode = code
	return nil
}

func (s *machineSessionSuite) setupUserSession(_ *tc.C, withPty bool, clientMessage string) {
	s.userSession = &userSession{
		isPty: withPty,
	}
	if withPty {
		s.userSession.stdin.Write([]byte(clientMessage))
	} else {
		s.userSession.clientCommand = clientMessage
	}
}

func (s *machineSessionSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *machineSessionSuite) TestMachineSessionProxy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isPty := true
	s.setupUserSession(c, isPty, "Hello from the client!\n")

	testServer := startTestServer(c)
	defer testServer.listener.Close()

	machineConn, err := testServer.listener.Dial()
	c.Assert(err, tc.ErrorIsNil)
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

	sessionHandler := sessionHandler{
		connector: s.mockConnector,
		modelType: model.IAAS,
	}

	err = sessionHandler.machineSessionProxy(s.userSession, virtualhostname.Info{})
	c.Check(err, tc.ErrorIsNil)
	c.Check(s.userSession.stdout.String(), tc.Equals, "Hello from the server!\r\n")
	c.Check(s.userSession.stderr.String(), tc.Equals, "An error from the server!\n")
	c.Check(string(testServer.serverRx), tc.Equals, "Hello from the client!\n")

	// Check the connection to the machine is closed.
	closeCheck, _ := machineConn.(*closeChecker)
	c.Assert(closeCheck, tc.NotNil)
	c.Check(closeCheck.closed.Load(), tc.Equals, true)
}

func (s *machineSessionSuite) TestMachineCommandProxy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isPty := false
	s.setupUserSession(c, isPty, "neovim")

	testServer := startTestServer(c)
	defer testServer.listener.Close()

	conn, err := testServer.listener.Dial()
	c.Assert(err, tc.ErrorIsNil)

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

	sessionHandler := sessionHandler{
		connector: s.mockConnector,
		modelType: model.IAAS,
	}

	err = sessionHandler.machineSessionProxy(s.userSession, virtualhostname.Info{})
	c.Check(err, tc.ErrorIsNil)
	c.Check(s.userSession.stdout.String(), tc.Equals, "No PTY requested.\n")
	c.Check(s.userSession.stderr.String(), tc.Equals, "An error from the server!\n")
	c.Check(string(testServer.serverRx), tc.Equals, "neovim")
}

func (s *machineSessionSuite) TestConnectToMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isPty := false
	s.setupUserSession(c, isPty, "neovim")

	s.mockConnector.EXPECT().Connect(gomock.Any()).DoAndReturn(
		func(destination virtualhostname.Info) (*gossh.Client, error) {
			return nil, errors.New("fake-connection-error")
		},
	)

	sessionHandler := sessionHandler{
		connector: s.mockConnector,
		modelType: model.IAAS,
		logger:    loggertesting.WrapCheckLog(c),
	}

	sessionHandler.Handle(s.userSession, virtualhostname.Info{})
	c.Check(s.userSession.exitCode, tc.Equals, 1)
	c.Check(s.userSession.stdout.String(), tc.Equals, "")
	c.Check(s.userSession.stderr.String(), tc.Equals, "failed to proxy machine session: fake-connection-error\n")
}
