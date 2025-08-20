// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// ErrBroken is returned when the connection to the remote relation caller
	// is broken.
	ErrBroken = errors.ConstError("remote relation caller connection broken")

	// ErrAPIRemoteRelationCallerDead is returned when the remote relation
	// caller is dead and cannot be used.
	ErrAPIRemoteRelationCallerDead = errors.ConstError("remote relation caller is dead")
)

// APIInfoGetter is an interface that provides a method to get the API
// information for a given model.
type APIInfoGetter interface {
	// GetAPIInfoForModel retrieves the API information for the specified model.
	GetAPIInfoForModel(ctx context.Context, modelUUID model.UUID) (api.Info, error)
}

// ConnectionGetter is an interface that provides a method to get a connection
// for a given model and API information.
type ConnectionGetter interface {
	// GetConnectionForModel returns the remote API connection for the
	// specified model. The connection must be valid for the lifetime of the
	// returned RemoteConnection.
	GetConnectionForModel(ctx context.Context, modelUUID model.UUID, apiInfo api.Info) (api.Connection, error)
}

// Config defines the configuration for the remote relation caller worker.
type Config struct {
	// APIInfoGetter is used to retrieve API information for models.
	APIInfoGetter APIInfoGetter

	// ConnectionGetter is a function that gets connections for models.
	ConnectionGetter ConnectionGetter

	// RestartDelay is the delay before the worker restarts after a failure.
	RestartDelay time.Duration

	Clock  clock.Clock
	Logger logger.Logger
}

type request struct {
	ModelName model.UUID
	Response  chan response
}

type response struct {
	Connection api.Connection
	Error      error
}

type remoteWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	apiInfoGetter    APIInfoGetter
	connectionGetter ConnectionGetter

	requests chan request
}

// NewWorker creates a new remote relation caller worker.
func NewWorker(config Config) (worker.Worker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "api-remote-relation-caller",
		IsFatal:       func(err error) bool { return false },
		ShouldRestart: internalworker.ShouldRunnerRestart,
		Clock:         config.Clock,
		Logger:        internalworker.WrapLogger(config.Logger),
		RestartDelay:  config.RestartDelay,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &remoteWorker{
		runner: runner,

		apiInfoGetter:    config.APIInfoGetter,
		connectionGetter: config.ConnectionGetter,

		requests: make(chan request),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "api-remote-relation-caller",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// Kill stops the worker and cleans up resources.
func (w *remoteWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait blocks until the worker has stopped.
func (w *remoteWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteWorker) Report() map[string]any {
	return w.runner.Report()
}

// GetConnectionForModel returns the remote API connection for the
// specified model. The connection must be valid for the lifetime of the
// returned RemoteConnection.
func (w *remoteWorker) GetConnectionForModel(ctx context.Context, modelName model.UUID) (api.Connection, error) {
	response := make(chan response)
	select {
	case <-w.catacomb.Dying():
		return nil, errors.Capture(ErrAPIRemoteRelationCallerDead)
	case <-ctx.Done():
		return nil, errors.Capture(ctx.Err())
	case w.requests <- request{ModelName: modelName, Response: response}:
	}

	select {
	case <-w.catacomb.Dying():
		return nil, errors.Capture(ErrAPIRemoteRelationCallerDead)
	case <-ctx.Done():
		return nil, errors.Capture(ctx.Err())
	case res := <-response:
		return res.Connection, res.Error
	}
}

func (w *remoteWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case req := <-w.requests:
			conn, err := w.getConnectionForModel(ctx, req.ModelName)

			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case req.Response <- response{
				Connection: conn,
				Error:      err,
			}:
			}
		}
	}
}

func (w *remoteWorker) getConnectionForModel(ctx context.Context, modelUUID model.UUID) (api.Connection, error) {
	ns := modelUUID.String()
	err := w.runner.StartWorker(ctx, ns, func(ctx context.Context) (worker.Worker, error) {
		apiInfo, err := w.apiInfoGetter.GetAPIInfoForModel(ctx, modelUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}

		conn, err := w.connectionGetter.GetConnectionForModel(ctx, modelUUID, apiInfo)
		if err != nil {
			return nil, errors.Capture(err)
		}

		return newConnectionWorker(apiInfo, conn), nil
	})
	if err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
		return nil, errors.Capture(err)
	}

	sub, err := w.runner.Worker(ns, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Capture(err)
	}

	return sub.(*connectionWorker).Connection(), nil
}

type connectionWorker struct {
	tomb tomb.Tomb

	apiInfo api.Info
	conn    api.Connection
}

func newConnectionWorker(apiInfo api.Info, conn api.Connection) *connectionWorker {
	w := connectionWorker{
		apiInfo: apiInfo,
		conn:    conn,
	}
	w.tomb.Go(w.loop)
	return &w
}

// Kill stops the connection worker.
func (w *connectionWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait blocks until the connection worker has stopped.
func (w *connectionWorker) Wait() error {
	return w.tomb.Wait()
}

// Connection returns the API connection for the remote relation caller.
func (w *connectionWorker) Connection() api.Connection {
	return w.conn
}

func (w *connectionWorker) Report() map[string]any {
	return map[string]any{
		"addresses":       w.apiInfo.Addrs,
		"controller-uuid": w.apiInfo.ControllerUUID,
		"model-uuid":      w.apiInfo.ModelTag.Id(),
	}
}

func (w *connectionWorker) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case <-w.conn.Broken():
			// If the connection is broken, trash the worker, which will force
			// a new connection to be created by the runner restarting.
			return ErrBroken
		}
	}
}
