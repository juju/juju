// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

type workerConfig struct {
	statePool               *state.StatePool
	controllerConfigGetter  ControllerConfigGetter
	mux                     *apiserverhttp.Mux
	clock                   clock.Clock
	newStateAuthenticatorFn NewStateAuthenticatorFunc
}

func (w workerConfig) Validate() error {
	if w.statePool == nil {
		return errors.NotValidf("empty statePool")
	}
	if w.controllerConfigGetter == nil {
		return errors.NotValidf("empty controllerConfigGetter")
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
	catacomb                catacomb.Catacomb
	cfg                     workerConfig
	authenticator           macaroon.LocalMacaroonAuthenticator
	managedCtrlConfigGetter *managedCtrlConfigGetter
}

func newWorker(cfg workerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := argsWorker{
		cfg:                     cfg,
		managedCtrlConfigGetter: newManagedCtrlConfigGetter(cfg.controllerConfigGetter),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.managedCtrlConfigGetter,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

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
	authenticator, err := w.cfg.newStateAuthenticatorFn(
		w.cfg.statePool,
		w.managedCtrlConfigGetter,
		w.cfg.mux,
		w.cfg.clock,
		w.catacomb.Dying(),
	)
	if err != nil {
		return errors.Trace(err)
	}
	w.authenticator = authenticator

	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

// managedCtrlConfigGetter is a ControllerConfigGetter that wraps another
// ControllerConfigGetter and cancels the context when the tomb is dying.
// This is because the location of the controller config request is not
// cancellable, so we need the ability to cancel the controller config
// request when the tomb is dying. This should prevent any lockup when the
// controller is shutting down.
type managedCtrlConfigGetter struct {
	tomb         tomb.Tomb
	configGetter ControllerConfigGetter
}

func newManagedCtrlConfigGetter(configGetter ControllerConfigGetter) *managedCtrlConfigGetter {
	w := &managedCtrlConfigGetter{
		configGetter: configGetter,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

// ControllerConfig is part of the ControllerConfigGetter interface.
func (b *managedCtrlConfigGetter) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return b.configGetter.ControllerConfig(b.tomb.Context(ctx))
}

// Kill is part of the worker.Worker interface.
func (b *managedCtrlConfigGetter) Kill() {
	b.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (b *managedCtrlConfigGetter) Wait() error {
	return b.tomb.Wait()
}
