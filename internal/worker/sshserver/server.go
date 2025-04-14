// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/canonical/lxd/shared/logger"
	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/virtualhostname"
	jujussh "github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/rpc/params"
)

type authenticatedViaPublicKey struct{}

// SessionHandler is an interface that proxies SSH sessions to a target unit/machine.
type SessionHandler interface {
	Handle(s ssh.Session, destination virtualhostname.Info)
}

// ServerWorkerConfig holds the configuration required by the server worker.
type ServerWorkerConfig struct {
	// Logger holds the logger for the server.
	Logger Logger

	// Listener holds a listener to provide the server. Should you wish to run
	// the server on a pre-existing listener, you can provide it here.
	// Otherwise, leave this value nil and a listener will be spawned.
	Listener net.Listener

	// JumpHostKey holds the host key for the jump server.
	JumpHostKey string

	// Port holds the port the server will listen on. If you provide your own
	// listener this can be left zeroed.
	Port int

	// MaxConcurrentConnections is the maximum number of concurrent connections
	// we accept for our ssh server.
	MaxConcurrentConnections int

	// NewSSHServerListener is a function that returns a listener and a
	// closeAllowed channel.
	NewSSHServerListener func(net.Listener, time.Duration) net.Listener

	// FacadeClient holds the SSH server's facade client.
	FacadeClient FacadeClient

	// disableAuth is a test-only flag that disables authentication.
	disableAuth bool

	// SessionHandler handles proxying SSH sessions to the target machine.
	SessionHandler SessionHandler
}

// Validate validates the workers configuration is as expected.
func (c ServerWorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.JumpHostKey == "" {
		return errors.NotValidf("empty JumpHostKey")
	}
	if c.NewSSHServerListener == nil {
		return errors.NotValidf("missing NewSSHServerListener")
	}
	if c.FacadeClient == nil {
		return errors.NotValidf("missing FacadeClient")
	}
	if c.SessionHandler == nil {
		return errors.NotValidf("missing SessionHandler")
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

	// concurrentConnections holds the number of concurrent connections.
	concurrentConnections atomic.Int32
}

// NewServerWorker returns a running embedded SSH server.
func NewServerWorker(config ServerWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	s := &ServerWorker{config: config}

	s.Server = s.NewJumpServer()

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

	listener := config.NewSSHServerListener(s.config.Listener, time.Second*10)

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
		// Keep the listener and the server alive until the tomb is killed.
		<-s.tomb.Dying()

		// Close the listener, this prevents a race in the test.
		if err := listener.Close(); err != nil {
			s.config.Logger.Errorf("failed to close listener: %v", err)
		}

		if err := s.Server.Close(); err != nil {
			// There's really not a lot we can do if the shutdown fails,
			// either due to a timeout or another reason. So we simply log it.
			s.config.Logger.Errorf("failed to shutdown server: %v", err)
			return errors.Trace(err)
		}

		return tomb.ErrDying
	})

	return s, nil
}

func (s *ServerWorker) NewJumpServer() *ssh.Server {
	server := ssh.Server{
		ConnCallback: s.connCallback(),
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			ctx.SetValue(authenticatedViaPublicKey{}, true)
			return true
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			return false
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": s.directTCPIPHandler,
		},
	}

	if s.config.disableAuth {
		server.PublicKeyHandler = nil
		server.PasswordHandler = nil
	}

	return &server
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
		return errors.Trace(err)
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
	info, err := virtualhostname.Parse(d.DestAddr)
	if err != nil {
		s.rejectChannel(newChan, "Failed to parse destination address")
		return
	}
	signer, err := s.hostKeySignerForTarget(info.String())
	if err != nil {
		s.rejectChannel(newChan, "Failed to get host key")
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		ch.Close()
		s.config.Logger.Errorf("failed to accept channel: %v", err)
		return
	}

	// gossh.Request are requests sent outside of the normal stream of data (ex. pty-req for an interactive session).
	// Since we only need the raw data to redirect, we can discard them.
	go gossh.DiscardRequests(reqs)

	server, err := s.newEmbeddedSSHServer(ctx, info)
	if err != nil {
		s.config.Logger.Errorf("failed to create embedded server: %v", err)
		ch.Close()
		return
	}

	server.AddHostKey(signer)
	server.HandleConn(newChannelConn(ch))
}

// hostKeySignerForTarget returns a signer for the target hostname, by calling the facade client.
func (s *ServerWorker) hostKeySignerForTarget(hostname string) (gossh.Signer, error) {
	key, err := s.config.FacadeClient.VirtualHostKey(params.SSHVirtualHostKeyRequestArg{Hostname: hostname})
	if err != nil {
		return nil, errors.Trace(err)
	}
	privateKey, err := jujussh.UnmarshalPrivateKey(key)
	if err != nil {
		return nil, errors.Trace(err)
	}

	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return signer, nil
}

// connCallback returns a connCallback function that limits the number of concurrent connections.
func (s *ServerWorker) connCallback() ssh.ConnCallback {
	return func(ctx ssh.Context, conn net.Conn) net.Conn {
		current := s.concurrentConnections.Add(1)
		if int(current) > s.config.MaxConcurrentConnections {
			// set the deadline because we don't want to block the connection to write an error.
			err := conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
			if err != nil {
				logger.Errorf("failed to set write deadline: %v", err)
			}
			_, err = conn.Write([]byte("too many connections.\n"))
			if err != nil {
				logger.Errorf("failed to write to connection: %v", err)
			}
			// The connection is close before returning, otherwise
			// the context is not cancelled and the counter is not decremented.
			conn.Close()
			s.concurrentConnections.Add(-1)
			return conn
		}
		go func() {
			<-ctx.Done()
			s.concurrentConnections.Add(-1)
		}()
		return conn
	}
}

// newEmbeddedSSHServer creates a new embedded SSH server for the given context and model info.
func (s *ServerWorker) newEmbeddedSSHServer(ctx ssh.Context, info virtualhostname.Info) (*ssh.Server, error) {
	authenticatedViaPublicKey, _ := ctx.Value(authenticatedViaPublicKey{}).(bool)
	var keysToVerify []gossh.PublicKey
	var err error
	// if the user is authenticated via public key, we need to verify the key
	// against the model's authorized keys.
	// if the user is not authenticated via public key, we need to verify the
	// key against the public keys in the jwt claims.
	if authenticatedViaPublicKey {
		sshPkiAuthArgs := params.ListAuthorizedKeysArgs{
			ModelUUID: info.ModelUUID(),
		}
		keysToVerify, err = s.config.FacadeClient.ListPublicKeysForModel(sshPkiAuthArgs)
		if err != nil {
			s.config.Logger.Errorf("failed to fetch public keys for model: %v", err)
			return nil, errors.Trace(err)
		}
	}

	forwardHandler := &ssh.ForwardedTCPHandler{}
	server := &ssh.Server{
		PublicKeyHandler: func(ctx ssh.Context, keyPresented ssh.PublicKey) bool {
			for _, key := range keysToVerify {
				if ssh.KeysEqual(key, keyPresented) {
					return true
				}
			}
			return false
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
		Handler: func(session ssh.Session) {
			s.config.SessionHandler.Handle(session, info)
		},
	}

	if s.config.disableAuth {
		server.PublicKeyHandler = nil
		server.PasswordHandler = nil
	}

	return server, nil
}

// Report returns a map of metrics from the server worker.
func (s *ServerWorker) Report() map[string]any {
	return map[string]any{
		"concurrent_connections": s.concurrentConnections.Load(),
	}
}

func (s *ServerWorker) rejectChannel(newChan gossh.NewChannel, reason string) {
	err := newChan.Reject(gossh.ConnectionFailed, reason)
	if err != nil {
		s.config.Logger.Errorf("failed to reject channel: %v", err)
	}
}
