// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
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
	"github.com/juju/juju/internal/sshconn"
	"github.com/juju/juju/internal/sshtunneler"
	jujussh "github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/rpc/params"
)

type connectionStartTime struct{}

// ProxyHandlers is an interface that provides methods handling SSH connections.
// These methods proxy connections to the target unit/machine.
type ProxyHandlers interface {
	SessionHandler(s ssh.Session)
	DirectTCPIPHandler() ssh.ChannelHandler
}

// ProxyFactory is an interface that creates new proxy handlers
// based on the target destination.
type ProxyFactory interface {
	New(ConnectionInfo) (ProxyHandlers, error)
}

type tunnelIDKey struct{}

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

	// JWTParser holds the JWT parser to use for auth.
	JWTParser JWTParser

	// disableAuth is a test-only flag that disables authentication.
	disableAuth bool

	// ProxyFactory creates objects that can proxy SSH connection.
	ProxyFactory ProxyFactory

	// TunnelTracker holds the tunnel tracker used to requests SSH
	// connections to machines.
	TunnelTracker *sshtunneler.Tracker

	// metricsCollector is collects Prometheus style metrics for the server.
	metricsCollector *Collector
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
	if c.ProxyFactory == nil {
		return errors.NotValidf("missing ProxyHandlers")
	}
	if c.JWTParser == nil {
		return errors.NotValidf("missing JWTParser")
	}
	if c.TunnelTracker == nil {
		return errors.NotValidf("missing TunnelTracker")
	}
	if c.metricsCollector == nil {
		return errors.NotValidf("missing metricsCollector")
	}
	return nil
}

// ServerWorker is a worker that runs an ssh server.
type ServerWorker struct {
	tomb tomb.Tomb

	// Server holds the running SSH server.
	Server *ssh.Server

	// authenticator holds the authenticator for the server.
	authenticator *authenticator

	// config holds the configuration required by the server worker.
	config ServerWorkerConfig

	// concurrentConnections holds the number of concurrent connections.
	concurrentConnections atomic.Int32
}

// NewServerWorker returns a worker with a running SSH server.
func NewServerWorker(config ServerWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	s := &ServerWorker{
		config: config,
	}

	s.authenticator = &authenticator{
		logger:        config.Logger,
		jwtParser:     config.JWTParser,
		facadeClient:  config.FacadeClient,
		tunnelTracker: config.TunnelTracker,
		metrics:       config.metricsCollector,
	}

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

// NewJumpServer creates a new SSH server with the given configuration.
func (s *ServerWorker) NewJumpServer() *ssh.Server {
	server := ssh.Server{
		ConnCallback: s.connCallback(),
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return s.authenticator.publicKeyAuthentication(ctx, key)
		},
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			return s.authenticator.passwordAuthentication(ctx, password)
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip":                s.directTCPIPHandler,
			sshtunneler.JujuTunnelChannel: s.reverseTunnelHandler,
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

// reverseTunnelHandler is a Juju specific SSH channel handler specifically
// for reverse SSH tunnels established from machines.
// These requests must always be associated with a tunnel ID obtained during
// authentication so that we can push the tunnel to the tunnel tracker.
//
// We need to take care to only close the connection in case of an error, otherwise we
// will be closing the connection before the object requesting the tunnel can use it.
func (s *ServerWorker) reverseTunnelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	tunnelID, _ := ctx.Value(tunnelIDKey{}).(string)
	if tunnelID == "" {
		s.config.Logger.Errorf("missing tunnel ID")
		_ = newChan.Reject(gossh.Prohibited, "missing tunnel ID")
		_ = conn.Close()
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		s.config.Logger.Errorf("failed to accept reverse tunnel channel creation request: %v", err)
		_ = conn.Close()
		return
	}

	go gossh.DiscardRequests(reqs)

	// The timeout here is intentionally short because we expect there to be
	// a routine waiting for the connection.
	pushCtx, cancelF := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelF()

	err = s.config.TunnelTracker.PushTunnel(pushCtx, tunnelID, sshconn.NewChannelConn(ch))
	if err != nil {
		s.config.Logger.Errorf("failed to push tunnel: %v", err)
		_ = conn.Close()
	}
}

// directTCPIPHandler handles a user's request to connect to a remote host, in our case, used
// to connect to the target unit/machine. We use this approach to accept routing information
// while being able to terminate the user's SSH connection at the Juju controller.
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
		s.config.Logger.Errorf("failed to get host key signer: %v", err)
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

	server, err := s.newTerminatingSSHServer(ctx, info)
	if err != nil {
		s.config.Logger.Errorf("failed to create embedded server: %v", err)
		ch.Close()
		return
	}

	server.AddHostKey(signer)
	server.HandleConn(sshconn.NewChannelConn(ch))
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
		ctx.SetValue(connectionStartTime{}, time.Now())
		current := s.concurrentConnections.Add(1)
		s.config.metricsCollector.connectionCount.Inc()

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
			// The connection is closed before returning, otherwise
			// the context is not cancelled and the counter is not decremented.
			conn.Close()
			s.config.metricsCollector.connectionCount.Dec()
			s.concurrentConnections.Add(-1)
			return conn
		}
		go func() {
			<-ctx.Done()
			s.config.metricsCollector.connectionCount.Dec()
			s.concurrentConnections.Add(-1)

			endTime, ok := ctx.Value(connectionStartTime{}).(time.Time)
			if ok {
				s.config.metricsCollector.connectionDuration.Observe(time.Since(endTime).Seconds())
			}
		}()
		return conn
	}
}

// newTerminatingSSHServer creates a new SSH server for the given context and model info
// that terminates the user's SSH connection and non-transparently proxies the traffic through
// to the final destination.
func (s *ServerWorker) newTerminatingSSHServer(ctx ssh.Context, destination virtualhostname.Info) (*ssh.Server, error) {
	// Note that the context we enter this function with is not
	// the context that will be used in the terminating server.
	var authenticator terminatingServerAuthenticator
	if !s.config.disableAuth {
		var err error
		authenticator, err = s.authenticator.newTerminatingServerAuthenticator(ctx, destination)
		if err != nil {
			return nil, err
		}
	}

	startTime, _ := ctx.Value(connectionStartTime{}).(time.Time)
	connInfo := ConnectionInfo{
		startTime:   startTime,
		destination: destination,
	}
	proxier, err := s.config.ProxyFactory.New(connInfo)
	if err != nil {
		return nil, err
	}

	server := &ssh.Server{
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return authenticator.PublicKeyAuthentication(ctx, key)
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": proxier.DirectTCPIPHandler(),
		},
		Handler: func(session ssh.Session) {
			proxier.SessionHandler(session)
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
