// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/state"
)

type workerConfig struct {
	statePool               *state.StatePool
	controllerConfigService ControllerConfigService
	userService             UserService
	mux                     *apiserverhttp.Mux
	clock                   clock.Clock
	newStateAuthenticatorFn NewStateAuthenticatorFunc
}

func (w workerConfig) Validate() error {
	if w.statePool == nil {
		return errors.NotValidf("empty statePool")
	}
	if w.controllerConfigService == nil {
		return errors.NotValidf("empty controllerConfigService")
	}
	if w.userService == nil {
		return errors.NotValidf("empty userService")
	}
	if w.mux == nil {
		return errors.NotValidf("empty mux")
	}
	if w.clock == nil {
		return errors.NotValidf("empty clock")
	}
	if w.newStateAuthenticatorFn == nil {
		return errors.NotValidf("empty newStateAuthenticatorFn")
	}
	return nil
}

type argsWorker struct {
	catacomb        catacomb.Catacomb
	cfg             workerConfig
	authenticator   macaroon.LocalMacaroonAuthenticator
	managedServices *managedServices
}

func newWorker(cfg workerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := argsWorker{
		cfg: cfg,
		managedServices: newManagedServices(
			cfg.controllerConfigService,
			cfg.userService,
		),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.managedServices,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	authenticator, err := w.cfg.newStateAuthenticatorFn(
		w.cfg.statePool,
		w.managedServices,
		w.managedServices,
		w.cfg.mux,
		w.cfg.clock,
		w.catacomb.Dying(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.authenticator = authenticator

	return &w, nil
}

// Kill is part of the worker.Worker interface.
func (w *argsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *argsWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *argsWorker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

// managedServices is a ControllerConfigService and a UserService that wraps
// the underlying services and cancels the context when the tomb is dying.
// This is because the location of the request is not cancellable, so we need
// the ability to cancel the request when the tomb is dying. This should
// prevent any lockup when the controller is shutting down.
type managedServices struct {
	tomb                    tomb.Tomb
	controllerConfigService ControllerConfigService
	userService             UserService
}

func newManagedServices(
	controllerConfigService ControllerConfigService,
	userService UserService,
) *managedServices {
	w := &managedServices{
		controllerConfigService: controllerConfigService,
		userService:             userService,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

// ControllerConfig is part of the ControllerConfigService interface.
func (b *managedServices) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return b.controllerConfigService.ControllerConfig(b.tomb.Context(ctx))
}

// GetUserByName is part of the UserService interface.
func (b *managedServices) GetUserByAuth(ctx context.Context, name string, password auth.Password) (coreuser.User, error) {
	return b.userService.GetUserByAuth(b.tomb.Context(ctx), name, password)
}

// GetUserByName is part of the UserService interface.
func (b *managedServices) GetUserByName(ctx context.Context, name string) (coreuser.User, error) {
	return b.userService.GetUserByName(b.tomb.Context(ctx), name)
}

// UpdateLastModelLogin updates the last login time for the user with the
// given name.
func (b *managedServices) UpdateLastModelLogin(ctx context.Context, name string, modelUUID coremodel.UUID) error {
	return b.userService.UpdateLastModelLogin(b.tomb.Context(ctx), name, modelUUID)
}

// Kill is part of the worker.Worker interface.
func (b *managedServices) Kill() {
	b.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (b *managedServices) Wait() error {
	return b.tomb.Wait()
}
