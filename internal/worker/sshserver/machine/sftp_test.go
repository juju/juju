// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"bytes"
	"io"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/sftp"
	gomock "go.uber.org/mock/gomock"
	gossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
)

type sftpSuite struct {
	mockConnector *MockSSHConnector
}

var _ = gc.Suite(&sftpSuite{})

// SftpHandler returns a handler for the SFTP subsystem
// either implmenting a real sftp server with an in-memory
// filesystem or exiting early with a specific exit code.
func SftpHandler(c *gc.C, exitEarly bool) func(s ssh.Session) {
	return func(sess ssh.Session) {
		if exitEarly {
			err := sess.Exit(3) //arbitrary non-zero exit code
			c.Check(err, jc.ErrorIsNil)
			return
		}
		server := sftp.NewRequestServer(
			sess,
			sftp.InMemHandler(),
		)
		// Logic based on the Gliderlab's sftp server example.
		err := server.Serve()
		if err == io.EOF {
			server.Close()
		} else if err != nil {
			c.Errorf("sftp server completed with error: %s\n", err)
		}
	}
}

// startTestMachineServer creates a server that emulates the
// SSH server of the target machine, running an sftp server.
// Defer the returned listener's Close() method to cleanup the server.
func (s *sftpSuite) startTestMachineServer(c *gc.C, exitEarly bool) *testServer {
	ts := &testServer{}
	ts.server = &ssh.Server{
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": SftpHandler(c, exitEarly),
		},
	}
	ts.listener = bufconn.Listen(1024)
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
	return ts
}

// startTestControllerServer creates a test server that behaves like
// the controller's terminating SSH server with an sftp handler.
func (s *sftpSuite) startTestControllerServer(_ *gc.C, handler ssh.SubsystemHandler) *testServer {
	ts := &testServer{}
	ts.server = &ssh.Server{
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": handler,
		},
	}
	ts.listener = bufconn.Listen(1024)
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
	return ts
}

func (s *sftpSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockConnector = NewMockSSHConnector(ctrl)
	return ctrl
}

// TestSFTPHandler tests the SFTP proxy handler
// by creating a file and then reading it back.
func (s *sftpSuite) TestSFTPHandler(c *gc.C) {
	defer s.setupMocks(c).Finish()

	handlers := Handlers{
		connector:   s.mockConnector,
		logger:      loggo.GetLogger("test"),
		destination: virtualhostname.Info{},
	}

	controllerServer := s.startTestControllerServer(c, handlers.SFTPHandler())
	defer controllerServer.listener.Close()

	machineServer := s.startTestMachineServer(c, false)
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

	sftpClient, err := sftp.NewClient(client)
	c.Assert(err, jc.ErrorIsNil)
	defer sftpClient.Close()

	// Test SFTP operations
	// Create a file
	file, err := sftpClient.Create("testfile.txt")
	c.Assert(err, jc.ErrorIsNil)

	_, err = file.Write([]byte("Hello, SFTP!"))
	c.Assert(err, jc.ErrorIsNil)

	err = file.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Read the file
	readFile, err := sftpClient.Open("testfile.txt")
	c.Assert(err, jc.ErrorIsNil)
	defer readFile.Close()

	data := bytes.Buffer{}
	_, err = readFile.WriteTo(&data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data.String(), gc.Equals, "Hello, SFTP!")
}

// TestSFTPHandlesErrorCode tests that the client
// receives exit codes from the sftp server correctly,
// ensuring they are proxied through the Juju controller.
func (s *sftpSuite) TestSFTPHandlesErrorCode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	handlers := Handlers{
		connector:   s.mockConnector,
		logger:      loggo.GetLogger("test"),
		destination: virtualhostname.Info{},
	}

	controllerServer := s.startTestControllerServer(c, handlers.SFTPHandler())
	defer controllerServer.listener.Close()

	exitEarly := true
	machineServer := s.startTestMachineServer(c, exitEarly)
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

	sshConn, sshChan, sshReqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	c.Assert(err, jc.ErrorIsNil)
	sshClient := gossh.NewClient(sshConn, sshChan, sshReqs)

	// Here we use OpenChannel instead of NewSession
	// in order to use the sessionReqs to watch for an exit signal.
	session, sessionReqs, err := sshClient.OpenChannel("session", nil)
	c.Assert(err, jc.ErrorIsNil)

	gotExitSignal := atomic.Bool{}
	reqsDone := make(chan struct{})
	go func() {
		for req := range sessionReqs {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			if req.Type == "exit-status" {
				var payload struct {
					Code uint32
				}
				err = gossh.Unmarshal(req.Payload, &payload)
				c.Check(err, jc.ErrorIsNil)
				if payload.Code == 3 {
					gotExitSignal.Store(true)
				}
			}
		}
		close(reqsDone)
	}()

	err = requestSubsystem(session, "sftp")
	c.Assert(err, jc.ErrorIsNil)

	<-reqsDone
	c.Assert(gotExitSignal.Load(), gc.Equals, true)
}
