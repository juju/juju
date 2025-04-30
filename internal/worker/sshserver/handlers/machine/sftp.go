// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"io"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

// SFTPHandler returns a handler for the SFTP subsystem.
// It requests an sftp subsystem on the target machine
// and proxies the user's session to it.
func (s *Handlers) SFTPHandler() ssh.SubsystemHandler {
	return func(session ssh.Session) {
		handleError := func(err error) {
			s.logger.Errorf("sftp proxy failure: %v", err)
			_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
			_ = session.Exit(1)
		}

		client, err := s.connector.Connect(s.destination)
		if err != nil {
			handleError(errors.Annotate(err, "failed to connect to machine"))
			return
		}
		defer client.Close()

		machineChannel, machineReqs, err := client.OpenChannel("session", nil)
		if err != nil {
			handleError(errors.Annotate(err, "failed to request session"))
			return
		}
		defer machineChannel.Close()
		go proxyReqs(session, machineReqs, s.logger)

		err = requestSubsystem(machineChannel, "sftp")
		if err != nil {
			handleError(errors.Annotate(err, "failed to request subsystem"))
			return
		}

		wg := sync.WaitGroup{}
		wg.Add(2)

		// Note that we don't close the session immediately after copying
		// in order to propagate the exit code back to the client.

		go func() {
			defer wg.Done()
			defer machineChannel.Close()
			_, _ = io.Copy(machineChannel, session)
		}()

		go func() {
			defer wg.Done()
			defer machineChannel.Close()
			_, _ = io.Copy(session, machineChannel)
		}()

		wg.Wait()
	}
}

// proxyReqs proxies SSH requests (things like signals, etc) from the
// connection to the machine back to the client, relaying only those messages
// that do not require a reply. This matches the behaviour of the x/crypto/ssh
// `DiscardRequests` function with the addition of the proxying logic.
// When the reqs channel is closed, this indicates that the connection
// to the machine has ended, so we also close the client's session.
//
// This ensures that the client receives the correct exit codes when
// proxying sftp connections. Otherwise, the client can successfully
// send/receive files but end up with a non-zero exit code.
func proxyReqs(session ssh.Session, reqs <-chan *gossh.Request, logger Logger) {
	for msg := range reqs {
		if msg.WantReply {
			// This handles keepalives and matches
			// OpenSSH's behaviour.
			if msg.WantReply {
				_ = msg.Reply(false, nil)
			}
			continue
		}
		_, err := session.SendRequest(msg.Type, false, msg.Payload)
		if err != nil {
			logger.Errorf("failed to send request %s: %v", msg.Type, err)
		}
	}
	session.Close()
}

type subsystemRequestMsg struct {
	Subsystem string
}

// requestSubsystem requests the association of a subsystem with the session on the remote host.
// A subsystem is a predefined command that runs in the background when the ssh session is initiated
func requestSubsystem(channel gossh.Channel, subsystem string) error {
	msg := subsystemRequestMsg{
		Subsystem: subsystem,
	}
	ok, err := channel.SendRequest("subsystem", true, gossh.Marshal(&msg))
	if err == nil && !ok {
		err = errors.New("ssh: subsystem request failed")
	}
	return err
}
