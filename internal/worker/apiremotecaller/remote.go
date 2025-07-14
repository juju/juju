// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
)

const (
	newChangeRequestError = errors.ConstError("new change request")
)

// RemoteConnection is an interface that represents a connection to a remote
// API server.
type RemoteConnection interface {
	// Connection returns the connection to the remote API server.
	Connection(context.Context, func(context.Context, api.Connection) error) error
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

	ControllerID string

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

	controllerID string
	info         *api.Info

	apiOpener api.OpenFunc

	logger logger.Logger
	clock  clock.Clock

	changes     chan []string
	connections chan chan api.Connection
}

// NewRemoteServer creates a new RemoteServer that will connect to the remote
// apiserver and pass on messages to the pubsub endpoint of that apiserver.
func NewRemoteServer(config RemoteServerConfig) RemoteServer {
	return newRemoteServer(config, nil)
}

func newRemoteServer(config RemoteServerConfig, internalStates chan string) RemoteServer {
	w := &remoteServer{
		controllerID:   config.ControllerID,
		info:           config.APIInfo,
		logger:         config.Logger,
		clock:          config.Clock,
		apiOpener:      config.APIOpener,
		changes:        make(chan []string),
		internalStates: internalStates,
		connections:    make(chan chan api.Connection),
	}
	w.tomb.Go(w.loop)
	return w
}

// Connection returns the current connection to the remote API server if it's
// available, otherwise it will send the connection once it has connected
// to the remote API server.
// No updates about the connection will be sent to the connection channel,
// instead it's up to the caller to watch for the broken connection and then
// reconnect if necessary.
func (w *remoteServer) Connection(ctx context.Context, fn func(context.Context, api.Connection) error) error {
	ch := make(chan api.Connection, 1)

	select {
	case <-w.tomb.Dying():
	case <-ctx.Done():
	case w.connections <- ch:
	}

	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case <-ctx.Done():
		return ctx.Err()
	case conn := <-ch:
		return w.callFunc(ctx, conn, fn)
	}
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

func (w *remoteServer) Report() map[string]any {
	report := make(map[string]any)
	report["controller-id"] = w.controllerID
	report["addresses"] = w.info.Addrs
	return report
}

type request struct {
	ctx       context.Context
	addresses []string
}

func (w *remoteServer) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	requests := make(chan request)
	w.tomb.Go(func() error {
		// When we receive a new change, we want to be able to cancel the current
		// connection attempt. The current setup is that it will dial indefinitely
		// until it has connected. The cancelling of the connection should not
		// affect the current connection, it should always remain in the same
		// state.
		var canceler context.CancelCauseFunc
		defer func() {
			// Clean up the canceler if it exists.
			if canceler != nil {
				canceler(context.Canceled)
			}
		}()

		// If the worker is dying, we need to cancel the current connection
		// attempt.
		// Note: do not use context.Done() here, as it will cause the worker
		// to die for the wrong cause (context.Canceled).
		for {
			select {
			case <-w.tomb.Dying():
				// There is no guarantee that we'll have a context to cancel,
				// to avoid panics, we need to check if the canceler is nil.
				if canceler != nil {
					canceler(context.Canceled)
				}
				return tomb.ErrDying
			case addresses := <-w.changes:
				// Cancel the current connection attempt and then proxy the
				// change through to the main loop.
				if canceler != nil {
					canceler(newChangeRequestError)
				}

				// Create a new context for the next connection attempt.
				var requestCtx context.Context
				requestCtx, canceler = context.WithCancelCause(ctx)

				// We might want to consider only sending a change after a
				// period of time, to avoid sending too many changes at once.
				select {
				case <-w.tomb.Dying():
					// We'll always have a canceler if we're dying, so we can
					// safely call it here.
					canceler(context.Canceled)
					return tomb.ErrDying
				case requests <- request{
					ctx:       requestCtx,
					addresses: addresses,
				}:
				}
			}
		}
	})

	var (
		connected  bool
		monitor    <-chan struct{}
		connection api.Connection

		channels []chan api.Connection
	)

	defer func() {
		if connection != nil {
			if err := connection.Close(); err != nil {
				w.logger.Errorf(ctx, "failed to close connection %q: %v", w.controllerID, err)
			}
		}
	}()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case request := <-requests:
			var (
				addresses = request.addresses
				rctx      = request.ctx
			)
			w.logger.Debugf(ctx, "addresses for %q have changed: %v", w.controllerID, addresses)

			// If the addresses already exist, we don't need to do anything.
			if connected && w.addressesAlreadyExist(addresses) {
				w.logger.Tracef(ctx, "addresses for %q have not changed", w.controllerID)
				continue
			}

			conn, err := w.connect(rctx, addresses)
			if errors.Is(err, newChangeRequestError) {
				continue
			} else if err != nil {
				// If we're dying we don't want to return an error, we just want
				// to die.
				select {
				case <-w.tomb.Dying():
					return tomb.ErrDying
				default:
					return err
				}
			}

			if connection != nil {
				if err := connection.Close(); err != nil {
					w.logger.Errorf(ctx, "failed to close connection %q: %v", w.controllerID, err)
				}
			}

			w.logger.Debugf(ctx, "connected to %s with addresses: %v", w.controllerID, addresses)

			connection = conn

			// Notify all the channels that are waiting for a connection.
			for _, ch := range channels {
				select {
				case <-w.tomb.Dying():
					return tomb.ErrDying
				case ch <- connection:
				}
			}

			// Reset the channels, we can guarantee that nothing else has
			// been added to this slice because it's only local to this loop,
			// and everything is sequenced.
			channels = nil

			monitor = conn.Broken()

			// We've successfully connected to the remote server, so update the
			// addresses.
			w.info.Addrs = addresses
			connected = true

			w.reportInternalState(stateChanged)

		case <-monitor:
			// If the connection is lost, force the worker to restart. We
			// won't attempt to reconnect here, just make the worker die.
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			default:
				return errors.Errorf("connection to %q has been lost", w.controllerID)
			}

		case ch := <-w.connections:
			// If we don't have a connection, we'll add the channel to the list
			// of channels that are waiting for a connection.
			if !connected {
				channels = append(channels, ch)
				continue
			}

			// We have one, send it!
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case ch <- connection:
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

func (w *remoteServer) connect(ctx context.Context, addresses []string) (api.Connection, error) {
	w.logger.Debugf(ctx, "connecting to %s with addresses: %v", w.controllerID, addresses)

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
			w.logger.Debugf(ctx, "failed to connect to %s attempt %d, with addresses %v: %v", w.controllerID, attempt, info.Addrs, err)
		},
		IsFatalError: func(err error) bool {
			// This is the only legitimist error that can be returned from the
			// connection attempt. Otherwise it should keep trying until the
			// controller comes up.
			return errors.Is(context.Cause(ctx), newChangeRequestError)
		},
		Attempts:    retry.UnlimitedAttempts,
		Delay:       1 * time.Second,
		MaxDelay:    time.Minute,
		BackoffFunc: retry.DoubleDelay,
		Stop:        ctx.Done(),
		Clock:       w.clock,
	})
	if errors.Is(context.Cause(ctx), newChangeRequestError) {
		return nil, newChangeRequestError
	} else if err != nil {
		return nil, err
	}

	return connection, nil
}

// callFunc calls the given function with the given connection. It will
// cancel the context if the connection is broken.
func (w *remoteServer) callFunc(ctx context.Context, conn api.Connection, fn func(context.Context, api.Connection) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer cancel()

		select {
		case <-w.tomb.Dying():
		case <-ctx.Done():
		case <-conn.Broken():
		}
	}()

	return fn(ctx, conn)
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
