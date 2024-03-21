// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsocket

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
)

// ManifoldConfig describes the dependencies required by the metricsocket worker.
type ManifoldConfig struct {
	ServiceFactoryName string
	Logger             Logger
	NewWorker          func(Config) (worker.Worker, error)
	NewSocketListener  func(socketlistener.Config) (SocketListener, error)
	SocketName         string

	// TODO (stickupkid): Delete me once permissions are in place.
	StateName string
}

// Manifold returns a Manifold that encapsulates the metricsocket worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
			config.ServiceFactoryName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker func")
	}
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if cfg.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (_ worker.Worker, err error) {
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err = getter.Get(cfg.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	var st *state.State
	_, st, err = stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Make sure we clean up state objects if an error occurs.
	defer func() {
		if err != nil {
			_ = stTracker.Done()
		}
	}()

	var serviceFactory servicefactory.ControllerServiceFactory
	if err = getter.Get(cfg.ServiceFactoryName, &serviceFactory); err != nil {
		return nil, errors.Trace(err)
	}

	var w worker.Worker
	w, err = cfg.NewWorker(Config{
		UserService: serviceFactory.User(),
		PermissionService: permissionService{
			state: st,
		},
		Logger:            cfg.Logger,
		SocketName:        cfg.SocketName,
		NewSocketListener: cfg.NewSocketListener,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// SocketListener describes a worker that listens on a unix socket.
type SocketListener interface {
	worker.Worker
}

// NewSocketListener is a function that creates a new socket listener.
func NewSocketListener(config socketlistener.Config) (SocketListener, error) {
	return socketlistener.NewSocketListener(config)
}

// TODO (stickupkid): Delete me once permissions are in place, this is just
// thin wrapper around the state to add user permissions.
type permissionService struct {
	state *state.State
}

func (p permissionService) AddUserPermission(ctx context.Context, username string, access permission.Access) error {
	model, err := p.state.Model()
	if err != nil {
		return errors.Annotate(err, "getting model")
	}

	if !names.IsValidUserName(username) {
		return errors.NotValidf("invalid username %q", username)
	}

	// This password doesn't matter, as we don't read from the state user.
	metricsPassword, err := password.RandomPassword()
	if err != nil {
		return errors.Annotatef(err, "generating random password")
	}

	_, err = p.state.AddUser(username, username, metricsPassword, userCreator)
	if err != nil {
		return errors.Annotate(err, "adding user")
	}

	_, err = model.AddUser(state.UserAccessSpec{
		User:      names.NewUserTag(username),
		CreatedBy: names.NewUserTag("controller@juju"),
		Access:    access,
	})
	if err != nil {
		return errors.Annotate(err, "adding user permission")
	}
	return nil
}
