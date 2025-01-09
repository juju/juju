// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"
)

// RemoteConnection is an interface that represents a connection to a remote
// API server.
type RemoteConnection interface {
	// Connection returns a channel that will be populated with the connection
	// to the remote API server. If the connection is lost or the remote server
	// is unreachable, the channel will be closed.
	Connection() (api.Connection, bool)

	// Tag returns the tag of the remote API server.
	Tag() names.Tag
}

// RemoteServer represents the public interface of the worker
// responsible modeling the remote API server.
type RemoteServer interface {
	worker.Worker
	RemoteConnection
	UpdateAddresses(addresses []string)
}

// RemoteServerConfig defines all the attributes that are needed for a
// RemoteServer.
type RemoteServerConfig struct {
	Clock  clock.Clock
	Logger logger.Logger

	Target names.Tag

	// APIInfo is initially populated with the addresses of the target machine.
	APIInfo *api.Info
}

// remoteServer is a worker that provides addresses for the target API server.
type remoteServer struct {
	tomb tomb.Tomb

	target names.Tag
	info   *api.Info

	logger logger.Logger
	clock  clock.Clock

	changes chan []string

	mu                sync.Mutex
	currentConnection api.Connection
}

// NewRemoteServer creates a new RemoteServer that will connect to the remote
// apiserver and pass on messages to the pubsub endpoint of that apiserver.
func NewRemoteServer(config RemoteServerConfig) RemoteServer {
	w := &remoteServer{
		target:  config.Target,
		info:    config.APIInfo,
		logger:  config.Logger,
		clock:   config.Clock,
		changes: make(chan []string),
	}
	w.tomb.Go(w.loop)
	return w
}

// Connection returns a channel that will be populated with the connection to
// the remote API server. If the connection is lost or the remote server is
// unreachable, the channel will be closed.
func (w *remoteServer) Connection() (api.Connection, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.currentConnection, w.currentConnection != nil
}

// Tag returns the tag of the remote API server.
func (w *remoteServer) Tag() names.Tag {
	return w.target
}

// UpdateAddresses will update the addresses held for the target API server.
func (w *remoteServer) UpdateAddresses(addresses []string) {
	select {
	case <-w.tomb.Dying():
		return
	case w.changes <- addresses:
	}
}

// Kill is part of the worker.Worker interface.
func (w *remoteServer) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *remoteServer) Wait() error {
	return w.tomb.Wait()
}

func (w *remoteServer) loop() error {
	defer w.closeCurrentConnection()

	ctx, cancel := w.scopedContext()
	defer cancel()

	var connected bool
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case addresses := <-w.changes:
			w.logger.Debugf("addresses for %q have changed: %v", w.target, addresses)

			// If the addresses already exist, we don't need to do anything.
			if connected && w.addressesAlreadyExist(addresses) {
				continue
			}

			if err := w.connect(ctx, addresses); err != nil {
				return err
			}

			// We've successfully connected to the remote server, so update the
			// addresses.
			w.info.Addrs = addresses
			connected = true
		}
	}
}

func (w *remoteServer) addressesAlreadyExist(addresses []string) bool {
	if len(addresses) != len(w.info.Addrs) {
		return false
	}

	for i, addr := range addresses {
		if addr != w.info.Addrs[i] {
			return false
		}
	}

	return true
}

func (w *remoteServer) connect(ctx context.Context, addresses []string) error {
	w.logger.Debugf("connecting to %s with addresses: %v", w.target, addresses)

	// Use temporary info until we're sure we can connect. If the addresses
	// are invalid, but the existing connection is still valid, we don't want
	// to close it.
	info := *w.info
	info.Addrs = addresses

	// Start connecting to the remote API server.
	var connection api.Connection
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			conn, err := api.Open(ctx, &info, dialOpts)
			if err != nil {
				return err
			}

			connection = conn
			return nil
		},
		NotifyFunc: func(err error, attempt int) {
			// This is normal behavior, so we don't need to log it as an error.
			w.logger.Debugf("failed to connect to %s attempt %d, with addresses %v: %v", w.target, attempt, info.Addrs, err)
		},
		Attempts:    retry.UnlimitedAttempts,
		Delay:       1 * time.Second,
		BackoffFunc: retry.DoubleDelay,
		Stop:        ctx.Done(),
		Clock:       w.clock,
	})
	if err != nil {
		return err
	}

	w.closeCurrentConnection()

	w.mu.Lock()
	w.currentConnection = connection
	w.mu.Unlock()

	return nil
}

// closeCurrentConnection will close the current connection if it exists.
// This is best effort and will not return an error.
func (w *remoteServer) closeCurrentConnection() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentConnection == nil {
		return
	}

	err := w.currentConnection.Close()
	if err != nil {
		w.logger.Errorf("failed to close connection %q: %v", w.target, err)
	}

	w.currentConnection = nil
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *remoteServer) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}

var dialOpts = api.DialOpts{
	DialAddressInterval: 20 * time.Millisecond,
	// If for some reason we are getting rate limited, there is a standard
	// five second delay before we get the login response. Ideally we need
	// to wait long enough for this response to get back to us.
	// Ideally the apiserver wouldn't be rate limiting connections from other
	// API servers, see bug #1733256.
	Timeout:    10 * time.Second,
	RetryDelay: 1 * time.Second,
}
