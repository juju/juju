// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"
)

// ServerWorkerConfig holds the configuration required by the server worker.
type ServerWorkerConfig struct {
	// Logger holds the logger for the server.
	Logger Logger

	// Listener holds a listener to provide the server. Should you wish to run
	// the server on a pre-existing listener, you can provide it here. Otherwise,
	// leave this value nil and a listener will be spawned.
	Listener net.Listener

	// JumpHostKey holds the host key for the jump server.
	JumpHostKey string

	// Port holds the port the server will listen on. If you provide your own listener
	// this can be left zeroed.
	Port int
}

// Validate validates the workers configuration is as expected.
func (c ServerWorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("Logger")
	}
	if c.JumpHostKey == "" {
		return errors.NotValidf("JumpHostKey")
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
func NewServerWorker(config ServerWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	s := &ServerWorker{config: config}
	s.Server = &ssh.Server{
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

	// Set hostkey.
	if err := s.setJumpServerHostKey(); err != nil {
		return nil, errors.Trace(err)
	}

	if s.config.Listener == nil {
		listenAddr := net.JoinHostPort("", strconv.Itoa(config.Port))
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, err
		}
		s.config.Listener = listener
	}

	listener, closeAllowed := newSSHServerListener(s.config.Listener)

	// Start server.
	s.tomb.Go(func() error {
		err := s.Server.Serve(listener)
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return errors.Trace(err)
	})

	// Handle server cleanup.
	s.tomb.Go(func() error {
		<-s.tomb.Dying()
		<-closeAllowed
		if err := s.Server.Close(); err != nil {
			// There's really not a lot we can do if the shutdown fails,
			// either due to a timeout or another reason. So we simply log it.
			s.config.Logger.Errorf("failed to shutdown server: %v", err)
			return errors.Trace(err)
		}
		return nil
	})

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

func (s *ServerWorker) setJumpServerHostKey() error {
	signer, err := gossh.ParsePrivateKey([]byte(s.config.JumpHostKey))
	if err != nil {
		return err
	}

	s.Server.AddHostKey(signer)
	return nil
}

func (s *ServerWorker) directTCPIPHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	d := struct {
		DestAddr string
		DestPort uint32
		SrcAddr  string
		SrcPort  uint32
	}{}

	if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
		err := newChan.Reject(gossh.ConnectionFailed, "Failed to parse channel data")
		if err != nil {
			s.config.Logger.Errorf("failed to reject channel: %v", err)
		}
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
		_, err := io.Copy(ch, jumpServerPipe)
		if err != nil {
			s.config.Logger.Errorf("failed to copy data from jump server to client: %v", err)
		}

		return nil
	})
	s.tomb.Go(func() error {
		defer ch.Close()
		defer jumpServerPipe.Close()
		defer terminatingServerPipe.Close()
		_, err := io.Copy(jumpServerPipe, ch)
		if err != nil {
			s.config.Logger.Errorf("failed to copy data from client to jump server: %v", err)
		}

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
	terminatingHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		s.config.Logger.Errorf("failed to generate host key: %v", err)
		return
	}
	signer, err := gossh.NewSignerFromKey(terminatingHostKey)
	if err != nil {
		s.config.Logger.Errorf("failed to create signer: %v", err)
		return
	}

	server.AddHostKey(signer)
	server.HandleConn(terminatingServerPipe)
}
