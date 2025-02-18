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
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"
)

// ServerWorkerConfig holds the configuration required by the server worker.
type ServerWorkerConfig struct {
	Logger Logger
	// Listener holds a listener to provide the server. Should you wish to run
	// the server on a pre-existing listener, you can provide it here. Otherwise,
	// leave this value nil and a listener will be spawned.
	Listener net.Listener
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

	// TODO(ale8k): Update later to use the host key from StateServingInfo
	terminatingHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate host key")
	}
	signer, err := gossh.NewSignerFromKey(terminatingHostKey)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create signer")
	}

	// Set hostkey
	s.Server.AddHostKey(signer)

	if s.config.Listener == nil {
		listenAddr := net.JoinHostPort("", strconv.Itoa(2229))
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, err
		}
		s.config.Listener = listener
	}

	listener := newAcceptOnceListener(s.config.Listener)

	// Start server
	s.tomb.Go(func() error {
		err := s.Server.Serve(listener)
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return err
	})

	// Handle server cleanup
	s.tomb.Go(func() error {
		<-s.tomb.Dying()
		<-listener.closeAllowed
		if err := s.Server.Close(); err != nil {
			// There's really not a lot we can do if the shutdown fails,
			// either due to a timeout or another reason. So we simply log it.
			s.config.Logger.Errorf("failed to shutdown server: %v", err)
			return err
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

// acceptOnceListener is required to prevent a race condition
// that can occur in tests.
//
// The SSH server tracks the listeners in use
// but if the server's close() method executes
// before we reach a safe point in the Serve() method
// then the server's map of listeners will be empty.
// A safe point to indicate the server is ready is
// right before we start accepting connections.
// Accept() will return with error if the underlying
// listener is already closed.
//
// As such, we ensure accept has been called at least once
// before allowing a close to take effect. The corresponding
// piece to this is to receive from the closeAllowed channel
// within your cleanup routine.
type acceptOnceListener struct {
	net.Listener
	// closeAllowed indicates when the server has reached
	// a safe point that it can be killed.
	closeAllowed chan struct{}
	once         *sync.Once
}

func newAcceptOnceListener(l net.Listener) acceptOnceListener {
	return acceptOnceListener{
		Listener:     l,
		closeAllowed: make(chan struct{}),
		once:         &sync.Once{},
	}
}

func (l acceptOnceListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		close(l.closeAllowed)
	})
	return l.Listener.Accept()
}
