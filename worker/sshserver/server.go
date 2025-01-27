// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"
)

// ServerWorkerConfig holds the configuration required by the server worker.
type ServerWorkerConfig struct {
	Logger Logger
}

// Validate validates the workers configuration is as expected.
func (c ServerWorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("Logger is required")
	}
	return nil
}

// ServerWorker is a worker that runs an ssh server.
type ServerWorker struct {
	tomb tomb.Tomb

	// Server holds the embedded server.
	Server *ssh.Server

	// config holds the configuration required by the server worker.
	config ServerWorkerConfig
}

// NewServerWorker returns a running embedded SSH server.
func NewServerWorker(config ServerWorkerConfig, startServer bool) (*ServerWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	s := &ServerWorker{config: config}
	s.Server = &ssh.Server{
		Addr: ":2223",
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			return true
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": s.directTCPIPHandler,
		},
	}

	// TODO(ale8k): Update later to use the host key from StateServingInfo
	terminatingHostKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	signer, _ := gossh.NewSignerFromKey(terminatingHostKey)

	s.Server.AddHostKey(signer)

	if startServer {
		s.tomb.Go(s.loop)
	}

	return s, nil
}

// Kill stops the server worker by killing the tomb. Implements worker.Worker.
func (s *ServerWorker) Kill() {
	s.tomb.Kill(nil)
}

// Wait waits for the server worker to stop. Implements worker.Worker.
func (s *ServerWorker) Wait() error {
	return s.tomb.Wait()
}

func (s *ServerWorker) loop() error {
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := s.Server.Shutdown(ctx); err != nil {
			// There's really not a lot we can do if the shutdown fails,
			// either due to a timeout or another reason. So we simply log it.
			//
			// TODO(ale8k): It would be nice to write it to the machine agent log.
			// How do we do this?
			s.config.Logger.Errorf("failed to shutdown server: %v", err)
		}
	}()
	return s.Server.ListenAndServe()
}

func (s *ServerWorker) directTCPIPHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	d := struct {
		DestAddr string
		DestPort uint32
		SrcAddr  string
		SrcPort  uint32
	}{}

	if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
		newChan.Reject(gossh.ConnectionFailed, "Failed to parse channel data")
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}

	// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
	// Since we only need the raw data to redirect, we can discard them.
	go gossh.DiscardRequests(reqs)

	jumpServerPipe, terminatingServerPipe := net.Pipe()

	s.tomb.Go(func() error {
		defer ch.Close()
		defer jumpServerPipe.Close()
		defer terminatingServerPipe.Close()
		io.Copy(ch, jumpServerPipe)

		return nil
	})
	s.tomb.Go(func() error {
		defer ch.Close()
		defer jumpServerPipe.Close()
		defer terminatingServerPipe.Close()
		io.Copy(jumpServerPipe, ch)

		return nil
	})

	forwardHandler := &ssh.ForwardedTCPHandler{}
	server := &ssh.Server{
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		},
		LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
			return true
		}),
		// ReversePortForwarding will not be supported.
		ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			return false
		}),
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": ssh.DirectTCPIPHandler,
		},
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
		},
		Handler: func(s ssh.Session) {
			_, _ = s.Write([]byte(fmt.Sprintf("Your final destination is: %s as user: %s\n", d.DestAddr, s.User())))
		},
	}

	// TODO(ale8k): Update later to generate host keys per unit.
	terminatingHostKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	signer, _ := gossh.NewSignerFromKey(terminatingHostKey)

	server.AddHostKey(signer)
	server.HandleConn(terminatingServerPipe)
}
