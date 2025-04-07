// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/state"
)

type workerConfig struct {
	statePool               *state.StatePool
	domainServicesGetter    DomainServicesGetter
	controllerConfigService ControllerConfigService
	accessService           AccessService
	macaroonService         MacaroonService
	mux                     *apiserverhttp.Mux
	clock                   clock.Clock
	newStateAuthenticatorFn NewStateAuthenticatorFunc
}

func (w workerConfig) Validate() error {
	if w.statePool == nil {
		return errors.NotValidf("empty statePool")
	}
	if w.domainServicesGetter == nil {
		return errors.NotValidf("empty domainServicesGetter")
	}
	if w.controllerConfigService == nil {
		return errors.NotValidf("empty controllerConfigService")
	}
	if w.accessService == nil {
		return errors.NotValidf("empty accessService")
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
			cfg.domainServicesGetter,
			cfg.controllerConfigService,
			cfg.accessService,
			cfg.macaroonService,
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

	systemState, err := w.cfg.statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerModelUUID := systemState.ModelUUID()

	authenticator, err := w.cfg.newStateAuthenticatorFn(
		w.catacomb.Context(context.Background()),
		w.cfg.statePool,
		coremodel.UUID(controllerModelUUID),
		w.managedServices,
		w.managedServices,
		w.managedServices,
		w.managedServices,
		w.cfg.mux,
		w.cfg.clock,
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

// managedServices is a ControllerConfigService and a AccessService that wraps
// the underlying services and cancels the context when the tomb is dying.
// This is because the location of the request is not cancellable, so we need
// the ability to cancel the request when the tomb is dying. This should
// prevent any lockup when the controller is shutting down.
type managedServices struct {
	tomb                    tomb.Tomb
	domainServicesGetter    DomainServicesGetter
	controllerConfigService ControllerConfigService
	accessService           AccessService
	macaroonService         MacaroonService
}

func newManagedServices(
	domainServicesGetter DomainServicesGetter,
	controllerConfigService ControllerConfigService,
	accessService AccessService,
	macaroonService MacaroonService,
) *managedServices {
	w := &managedServices{
		domainServicesGetter:    domainServicesGetter,
		controllerConfigService: controllerConfigService,
		accessService:           accessService,
		macaroonService:         macaroonService,
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

// GetUserByAuth is part of the AccessService interface.
func (b *managedServices) GetUserByAuth(ctx context.Context, name coreuser.Name, password auth.Password) (coreuser.User, error) {
	return b.accessService.GetUserByAuth(b.tomb.Context(ctx), name, password)
}

// GetUserByName is part of the AccessService interface.
func (b *managedServices) GetUserByName(ctx context.Context, name coreuser.Name) (coreuser.User, error) {
	return b.accessService.GetUserByName(b.tomb.Context(ctx), name)
}

// ReadUserAccessLevelForTarget returns the user access level for the given
// user on the given target. A NotValid error is returned if the subject
// (user) string is empty, or the target is not valid. Any errors from the
// state layer are passed through. If the access level of a user cannot be
// found then [accesserrors.AccessNotFound] is returned.
func (b *managedServices) ReadUserAccessLevelForTarget(
	ctx context.Context, subject coreuser.Name, target permission.ID,
) (permission.Access, error) {
	return b.accessService.ReadUserAccessLevelForTarget(b.tomb.Context(ctx), subject, target)
}

// EnsureExternalUserIfAuthorized checks if an external user is missing from the
// database and has permissions on an object. If they do then they will be
// added. This ensures that juju has a record of external users that have
// inherited their permissions from everyone@external.
func (b *managedServices) EnsureExternalUserIfAuthorized(
	ctx context.Context, subject coreuser.Name, target permission.ID,
) error {
	return b.accessService.EnsureExternalUserIfAuthorized(b.tomb.Context(ctx), subject, target)
}

// UpdateLastModelLogin updates the last login time for the user with the
// given name.
func (b *managedServices) UpdateLastModelLogin(ctx context.Context, name coreuser.Name, modelUUID coremodel.UUID) error {
	return b.accessService.UpdateLastModelLogin(b.tomb.Context(ctx), name, modelUUID)
}

// GetLocalUsersKey returns the key pair used with the local users bakery.
func (b *managedServices) GetLocalUsersKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.macaroonService.GetLocalUsersKey(b.tomb.Context(ctx))
}

// GetLocalUsersThirdPartyKey returns the third party key pair used with the local users bakery.
func (b *managedServices) GetLocalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.macaroonService.GetLocalUsersThirdPartyKey(b.tomb.Context(ctx))
}

// GetExternalUsersThirdPartyKey returns the third party key pair used with the external users bakery.
func (b *managedServices) GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error) {
	return b.macaroonService.GetExternalUsersThirdPartyKey(b.tomb.Context(ctx))
}

// GetKeyContext (dbrootkeystore.GetKeyContext) gets the key
// with a given id.
//
// To satisfy dbrootkeystore.ContextBacking specification,
// if not key is found, a bakery.ErrNotFound error is returned.
func (b *managedServices) GetKeyContext(ctx context.Context, id []byte) (dbrootkeystore.RootKey, error) {
	return b.macaroonService.GetKeyContext(b.tomb.Context(ctx), id)
}

// FindLatestKeyContext (dbrootkeystore.FindLatestKeyContext) returns
// the most recently created root key k following all
// the conditions:
//
// k.Created >= createdAfter
// k.Expires >= expiresAfter
// k.Expires <= expiresBefore
//
// To satisfy dbrootkeystore.FindLatestKeyContext specification,
// if no such key is found, the zero root key is returned with a
// nil error
func (b *managedServices) FindLatestKeyContext(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (dbrootkeystore.RootKey, error) {
	return b.macaroonService.FindLatestKeyContext(b.tomb.Context(ctx), createdAfter, expiresAfter, expiresBefore)
}

// InsertKeyContext (dbrootkeystore.InsertKeyContext) inserts
// the given root key into state. If a key with matching
// id already exists, return a macaroonerrors.KeyAlreadyExists error.
func (b *managedServices) InsertKeyContext(ctx context.Context, key dbrootkeystore.RootKey) error {
	return b.macaroonService.InsertKeyContext(b.tomb.Context(ctx), key)
}

// GetAgentPasswordServiceForModel returns a AgentPasswordService for the given
// model.
func (b *managedServices) GetAgentPasswordServiceForModel(ctx context.Context, modelUUID coremodel.UUID) (authentication.AgentPasswordService, error) {
	services, err := b.ServicesForModel(b.tomb.Context(ctx), modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return services.AgentPassword(), nil
}

// ServicesForModel returns a DomainServices for the given model.
func (b *managedServices) ServicesForModel(ctx context.Context, modelID coremodel.UUID) (services.DomainServices, error) {
	return b.domainServicesGetter.ServicesForModel(b.tomb.Context(ctx), modelID)
}

// Kill is part of the worker.Worker interface.
func (b *managedServices) Kill() {
	b.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (b *managedServices) Wait() error {
	return b.tomb.Wait()
}
