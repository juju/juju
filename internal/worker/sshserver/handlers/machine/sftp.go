// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
)

// SFTPHandler proxies the SFTP subsystem to the target machine.
func (h *Handlers) SFTPHandler() ssh.SubsystemHandler {
	return func(session ssh.Session) {
		handleError := func(err error) {
			h.logger.Errorf(session.Context(), "SFTP proxy failure: %v", err)
			_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))
			_ = session.Exit(1)
		}

		client, err := h.connector.Connect(session.Context(), h.destination)
		if err != nil {
			handleError(errors.Annotate(err, "connecting to machine"))
			return
		}
		defer client.Close()

		machineChannel, machineRequests, err := client.OpenChannel("session", nil)
		if err != nil {
			handleError(errors.Annotate(err, "opening machine session"))
			return
		}
		defer machineChannel.Close()

		// Note that we don't call `session.Close()` in this function.
		// The routine copying *from* session will return once the
		// session is closed. Closing session is handled by
		// `proxyRequests` in order to propagate the exit code
		// back to the client.

		stop := context.AfterFunc(session.Context(), func() {
			_ = machineChannel.Close()
		})
		defer stop()

		// This routine is cleaned up when machineRequests channel closes
		// which occurs when machineChannel is closed.
		go proxyRequests(session, machineRequests, h.logger)

		if err := requestSubsystem(machineChannel, "sftp"); err != nil {
			handleError(errors.Annotate(err, "requesting SFTP subsystem"))
			return
		}

		proxy(machineChannel, session)
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
func proxyRequests(session ssh.Session, requests <-chan *gossh.Request, logger logger.Logger) {
	for request := range requests {
		if request.WantReply {
			// This handles keepalives and matches OpenSSH's behaviour.
			_ = request.Reply(false, nil)
			continue
		}
		if _, err := session.SendRequest(request.Type, false, request.Payload); err != nil {
			logger.Errorf(session.Context(), "sending SFTP request %q: %v", request.Type, err)
		}
	}
	_ = session.Close()
}

type subsystemRequest struct {
	Subsystem string
}

// requestSubsystem requests the association of a subsystem with the session on the remote host.
// A subsystem is a predefined command that runs in the background when the ssh session is initiated
func requestSubsystem(channel gossh.Channel, subsystem string) error {
	ok, err := channel.SendRequest("subsystem", true, gossh.Marshal(&subsystemRequest{Subsystem: subsystem}))
	if err == nil && !ok {
		return errors.New("ssh: subsystem request failed")
	}
	return err
}
