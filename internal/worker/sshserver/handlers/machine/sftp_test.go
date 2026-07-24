// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"io"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *machineSuite) TestSFTPHandler(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": func(session ssh.Session) {
			server := sftp.NewRequestServer(session, sftp.InMemHandler())
			if err := server.Serve(); err == io.EOF {
				_ = server.Close()
			}
		},
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": handlers.SFTPHandler(),
	}})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	c.Assert(err, tc.ErrorIsNil)
	defer sftpClient.Close()

	file, err := sftpClient.Create("testfile.txt")
	c.Assert(err, tc.ErrorIsNil)

	_, err = file.Write([]byte("Hello, SFTP!"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(file.Close(), tc.ErrorIsNil)

	file, err = sftpClient.Open("testfile.txt")
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	contents, err := io.ReadAll(file)

	c.Check(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "Hello, SFTP!")
}

func (s *machineSuite) TestSFTPHandlerProxiesExitStatus(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machine := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": func(session ssh.Session) { _ = session.Exit(3) },
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": handlers.SFTPHandler(),
	}})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	channel, requests, err := client.OpenChannel("session", nil)
	c.Assert(err, tc.ErrorIsNil)
	defer channel.Close()

	exitCode := make(chan uint32, 1)
	requestsDone := make(chan struct{})
	go func() {
		defer close(requestsDone)
		for request := range requests {
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
			if request.Type != "exit-status" {
				continue
			}
			var payload struct{ Code uint32 }
			if err := gossh.Unmarshal(request.Payload, &payload); err == nil {
				exitCode <- payload.Code
			}
		}
	}()

	c.Assert(requestSubsystem(channel, "sftp"), tc.ErrorIsNil)

	<-requestsDone
	c.Check(<-exitCode, tc.Equals, uint32(3))
}

func (s *machineSuite) TestSFTPHandlerProxiesMachineEOF(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machineClose := make(chan struct{})
	defer close(machineClose)
	machine := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": func(session ssh.Session) {
			_, _ = session.Write([]byte("response"))
			_ = session.CloseWrite()
			<-machineClose
		},
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": handlers.SFTPHandler(),
	}})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)
	defer client.Close()

	channel, _, err := client.OpenChannel("session", nil)
	c.Assert(err, tc.ErrorIsNil)
	defer channel.Close()
	c.Assert(requestSubsystem(channel, "sftp"), tc.ErrorIsNil)

	response := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(channel)
		response <- data
	}()

	select {
	case data := <-response:
		c.Check(data, tc.DeepEquals, []byte("response"))
	case <-c.Context().Done():
		c.Fatal("machine EOF was not proxied to the client")
	}
}

func (s *machineSuite) TestSFTPHandlerClosesMachineClientWhenClientDisconnects(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	machineSessionStarted := make(chan struct{})
	machineSessionStopped := make(chan struct{})
	machine := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": func(session ssh.Session) {
			close(machineSessionStarted)
			<-session.Context().Done()
			close(machineSessionStopped)
		},
	}})

	handlers, err := NewHandlers(destination, connectorForServer(machine), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	controller := startSSHTestServer(c, &ssh.Server{SubsystemHandlers: map[string]ssh.SubsystemHandler{
		"sftp": handlers.SFTPHandler(),
	}})

	client, err := controller.client()
	c.Assert(err, tc.ErrorIsNil)

	channel, _, err := client.OpenChannel("session", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(requestSubsystem(channel, "sftp"), tc.ErrorIsNil)

	select {
	case <-machineSessionStarted:
	case <-c.Context().Done():
		c.Fatal("machine SFTP session did not start")
	}

	c.Assert(client.Close(), tc.ErrorIsNil)

	select {
	case <-machineSessionStopped:
	case <-c.Context().Done():
		c.Fatal("machine SFTP session was not closed after client disconnect")
	}
}
