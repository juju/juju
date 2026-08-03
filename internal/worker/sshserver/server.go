// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/virtualhostname"
)

// SessionHandler is an interface that proxies SSH sessions to a target unit/machine.
type SessionHandler interface {
	// Handle proxies the SSH session to the target unit/machine.
	Handle(s ssh.Session, destination virtualhostname.Info)
}

// Authenticator authenticates jump SSH connections.
type Authenticator interface {
	// PublicKeyAuthentication authenticates a jump SSH connection using a public key.
	// Returns true if the public key is valid for the user.
	// Handles auth for user public keys.
	PublicKeyAuthentication(ssh.Context, ssh.PublicKey) (bool, error)
	// PasswordAuthentication authenticates a jump SSH connection using a password.
	// Returns true if the password is valid for the user.
	// Handles auth for external-auth and reverse-tunnel connections.
	PasswordAuthentication(ssh.Context, string) (bool, error)
}

// Authorizer checks whether an authenticated user may access a destination.
type Authorizer interface {
	// Authorize checks whether the authenticated user in the SSH connection
	// context is allowed to access the target destination.
	Authorize(ssh.Context, virtualhostname.Info) (bool, error)
}

// ServerWorkerConfig holds the configuration required by the server worker.
type ServerWorkerConfig struct {
	// Logger holds the logger for the server.
	Logger logger.Logger
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

	// SSHService resolves terminating SSH host keys for virtual destinations.
	SSHService SSHService
	// Authenticator authenticates jump and terminating SSH connections.
	Authenticator Authenticator
	// Authorizer checks whether an authenticated user may access a destination.
	Authorizer Authorizer
	// ProxyFactory creates target-specific session, forwarding, and SFTP handlers.
	ProxyFactory ProxyFactory
	// TunnelTracker accepts incoming reverse SSH tunnel connections.
	TunnelTracker TunnelTracker
}

// Validate validates the workers configuration is as expected.
func (c ServerWorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.JumpHostKey == "" {
		return errors.NotValidf("empty JumpHostKey")
	}
	if c.SSHService == nil {
		return errors.NotValidf("missing SSHService")
	}
	if c.Authenticator == nil {
		return errors.NotValidf("missing Authenticator")
	}
	if c.Authorizer == nil {
		return errors.NotValidf("missing Authorizer")
	}
	if c.ProxyFactory == nil {
		return errors.NotValidf("missing ProxyFactory")
	}
	if c.TunnelTracker == nil {
		return errors.NotValidf("missing TunnelTracker")
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

	// Delete the below once https://github.com/gliderlabs/ssh/pull/248
	// lands and we upgrade to the latest gliderlabs/ssh.
	closeAllowed, listener := newSyncSSHServerListener(s.config.Listener)

	s.tomb.Go(func() error {
		// Start server.
		s.tomb.Go(func() error {
			err := s.Server.Serve(listener)
			if errors.Is(err, ssh.ErrServerClosed) {
				return nil
			}
			return errors.Trace(err)
		})

		// Handle server cleanup.
		// Keep the listener and the server alive until the tomb is killed.
		<-s.tomb.Dying()

		// Wait on the closeAllowed (or a 5 second timeout) to indicate when
		// we can stop the SSH server.
		// This ensures we don't face a race condition, closing the server
		// too quickly after it was started, see listener.go for more details.
		select {
		case <-closeAllowed:
		case <-time.After(5 * time.Second):
		}
		if err := s.Server.Close(); err != nil {
			// There's really not a lot we can do if the shutdown fails,
			// either due to a timeout or another reason. So we simply log it.
			s.config.Logger.Errorf(context.TODO(), "failed to shutdown server: %v", err)
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
			ok, err := s.config.Authenticator.PublicKeyAuthentication(ctx, key)
			if err != nil {
				s.config.Logger.Warningf(ctx, "failed to authenticate public key: %v", err)
				return false
			}
			return ok
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			ok, err := s.config.Authenticator.PasswordAuthentication(ctx, password)
			if err != nil {
				s.config.Logger.Warningf(ctx, "failed to authenticate password: %v", err)
				return false
			}
			return ok
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			// Handle direct-tcpip channels for jump server connections from users.
			"direct-tcpip": s.directTCPIPHandler,
			// Handle reverse-tunnel channels for machine sshsession workers.
			coressh.JujuTunnelChannel: s.reverseTunnelHandler,
		},
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
			s.config.Logger.Errorf(ctx, "failed to reject channel: %v", err)
		}
		return
	}

	destination, err := virtualhostname.Parse(d.DestAddr)
	if err != nil {
		s.rejectChannel(ctx, newChan, "Failed to parse destination address")
		return
	}

	if ok, err := s.config.Authorizer.Authorize(ctx, destination); err != nil {
		s.config.Logger.Errorf(ctx, "failed to authorize access to destination: %v", err)
		s.rejectChannel(ctx, newChan, fmt.Sprintf("failed to authorize access to destination: %v", err))
		return
	} else if !ok {
		s.rejectChannel(ctx, newChan, "unauthorized")
		return
	}

	server, err := s.newTerminatingSSHServer(ctx, destination)
	if err != nil {
		s.config.Logger.Errorf(ctx, "failed to create embedded server: %v", err)
		s.rejectChannel(ctx, newChan, fmt.Sprintf("failed to create embedded server: %v", err))
		return
	}

	terminatingHostKey, err := s.config.SSHService.VirtualHostKey(ctx, destination)
	if err != nil {
		s.config.Logger.Errorf(ctx, "failed to resolve host key: %v", err)
		s.rejectChannel(ctx, newChan, fmt.Sprintf("failed to resolve host key: %v", err))
		return
	}
	signer, err := gossh.ParsePrivateKey([]byte(terminatingHostKey))
	if err != nil {
		s.config.Logger.Errorf(ctx, "failed to parse host key: %v", err)
		s.rejectChannel(ctx, newChan, fmt.Sprintf("failed to parse host key: %v", err))
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		s.config.Logger.Errorf(ctx, "accepting direct-tcpip channel: %v", err)
		return
	}

	// gossh.Request are requests sent outside of the normal stream of data (for
	// example, pty-req for an interactive session). The jump server only needs
	// the raw data channel, so it can discard these requests.
	go gossh.DiscardRequests(reqs)

	server.AddHostKey(signer)
	server.HandleConn(newChannelConn(ch))
}

// reverseTunnelHandler accepts a reverse SSH tunnel established by a machine
// sshsession worker. Ownership of a successfully pushed connection transfers
// to the tunnel tracker.
func (s *ServerWorker) reverseTunnelHandler(_ *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	tunnelID, _ := ctx.Value(tunnelIDKey{}).(string)
	if tunnelID == "" {
		s.rejectChannel(ctx, newChan, "missing tunnel ID")
		return
	}

	channel, requests, err := newChan.Accept()
	if err != nil {
		s.config.Logger.Errorf(ctx, "accepting reverse tunnel channel: %v", err)
		return
	}
	go gossh.DiscardRequests(requests)

	pushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.config.TunnelTracker.PushTunnel(pushCtx, tunnelID, newChannelConn(channel)); err != nil {
		s.config.Logger.Errorf(ctx, "pushing reverse tunnel: %v", err)
		_ = channel.Close()
	}
}

// connCallback returns a connCallback function that limits the number of concurrent connections.
func (s *ServerWorker) connCallback() ssh.ConnCallback {
	return func(ctx ssh.Context, conn net.Conn) net.Conn {
		current := s.concurrentConnections.Add(1)

		if int(current) > s.config.MaxConcurrentConnections {
			// set the deadline because we don't want to block the connection to write an error.
			err := conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
			if err != nil {
				s.config.Logger.Errorf(context.TODO(), "failed to set write deadline: %v", err)
			}
			_, err = conn.Write([]byte("too many connections.\n"))
			if err != nil {
				s.config.Logger.Errorf(context.TODO(), "failed to write to connection: %v", err)
			}
			// The connection is closed before returning, otherwise
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

// newTerminatingSSHServer creates an embedded SSH server that terminates the
// user's SSH connection and proxies it to the routed target.
func (s *ServerWorker) newTerminatingSSHServer(_ ssh.Context, destination virtualhostname.Info) (*ssh.Server, error) {
	handlers, err := s.config.ProxyFactory.New(destination)
	if err != nil {
		return nil, errors.Trace(err)
	}

	server := &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": handlers.DirectTCPIPHandler(),
		},
		Handler: func(session ssh.Session) {
			handlers.SessionHandler(session)
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": handlers.SFTPHandler(),
		},
	}

	return server, nil
}

// Report returns a map of metrics from the server worker.
func (s *ServerWorker) Report(ctx context.Context) map[string]any {
	return map[string]any{
		"concurrent_connections": s.concurrentConnections.Load(),
	}
}

func (s *ServerWorker) rejectChannel(ctx context.Context, newChan gossh.NewChannel, reason string) {
	err := newChan.Reject(gossh.ConnectionFailed, reason)
	if err != nil {
		s.config.Logger.Errorf(ctx, "failed to reject channel: %v", err)
	}
}
