// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
)

// RemoteConnection is an interface that represents a connection to a remote
// API server.
type RemoteConnection interface {
	// Connection returns the connection to the remote API server.
	Connection() api.Connection

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

	// APIOpener is a function that will open a connection to the target API
	// server.
	APIOpener api.OpenFunc
}

// remoteServer is a worker that provides addresses for the target API server.
type remoteServer struct {
	internalStates chan string
	tomb           tomb.Tomb

	target names.Tag
	info   *api.Info

	apiOpener api.OpenFunc

	logger logger.Logger
	clock  clock.Clock

	changes chan []string

	mu                sync.Mutex
	currentConnection api.Connection
}

// NewRemoteServer creates a new RemoteServer that will connect to the remote
// apiserver and pass on messages to the pubsub endpoint of that apiserver.
func NewRemoteServer(config RemoteServerConfig) RemoteServer {
	return newRemoteServer(config, nil)
}

func newRemoteServer(config RemoteServerConfig, internalStates chan string) RemoteServer {
	w := &remoteServer{
		target:         config.Target,
		info:           config.APIInfo,
		logger:         config.Logger,
		clock:          config.Clock,
		apiOpener:      config.APIOpener,
		changes:        make(chan []string),
		internalStates: internalStates,
	}
	w.tomb.Go(w.loop)
	return w
}

// Connection returns the connection to the remote API server.
func (w *remoteServer) Connection() api.Connection {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.currentConnection
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
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	defer w.closeCurrentConnection()

	ctx, cancel := w.scopedContext()
	defer cancel()

	// When we receive a new change, we want to be able to cancel the current
	// connection attempt. The current setup is that it will dial indefinitely
	// until it has connected. The cancelling of the connection should not
	// affect the current connection, it should always remain in the same
	// state.
	connectCtx, connectCancel := context.WithCancel(ctx)
	defer connectCancel()

	changes := make(chan []string)
	w.tomb.Go(func() error {
		// If the worker is dying, or the original context is cancelled, we
		// should return immediately.
		for {
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case <-ctx.Done():
				return ctx.Err()
			case change := <-w.changes:
				// Cancel the current connection attempt and then proxy the
				// change through to the main loop.
				connectCancel()

				// We might want to consider only sending a change after a
				// period of time, to avoid sending too many changes at once.
				select {
				case <-w.tomb.Dying():
					return tomb.ErrDying
				case changes <- change:
				}
			}
		}
	})

	var (
		connected bool
		monitor   <-chan struct{}
	)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case addresses := <-changes:
			w.logger.Debugf("addresses for %q have changed: %v", w.target, addresses)

			// If the addresses already exist, we don't need to do anything.
			if connected && w.addressesAlreadyExist(addresses) {
				continue
			}

			var err error
			monitor, err = w.connect(connectCtx, addresses)
			if err != nil {
				return err
			}

			// We've successfully connected to the remote server, so update the
			// addresses.
			w.info.Addrs = addresses
			connected = true

		case <-monitor:
			// If the connection is lost, force the worker to restart. We
			// won't attempt to reconnect here, just make the worker die.
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			default:
				return errors.Errorf("connection to %q has been lost", w.target)
			}
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

func (w *remoteServer) connect(ctx context.Context, addresses []string) (<-chan struct{}, error) {
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
			conn, err := w.apiOpener(ctx, &info, dialOpts)
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
		MaxDelay:    time.Minute,
		BackoffFunc: retry.DoubleDelay,
		Stop:        ctx.Done(),
		Clock:       w.clock,
	})
	if err != nil {
		return nil, err
	}

	w.closeCurrentConnection()

	w.mu.Lock()
	w.currentConnection = connection
	w.mu.Unlock()

	return connection.Broken(), nil
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

func (w *remoteServer) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
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
