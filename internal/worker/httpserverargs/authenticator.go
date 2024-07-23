// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/state"
)

// ControllerConfigService is an interface that can be implemented by
// types that can return a controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// GetUserByAuth returns the user with the given name and password.
	GetUserByAuth(ctx context.Context, name string, password auth.Password) (coreuser.User, error)

	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name string) (coreuser.User, error)

	// UpdateLastModelLogin updates the last login time for the user with the
	// given name on the given model.
	UpdateLastModelLogin(ctx context.Context, name string, modelUUID coremodel.UUID) error

	// ReadUserAccessLevelForTargetAddingMissingUser returns the user access level for
	// the given user on the given target. If the user is external and does not yet
	// exist, it is created. An accesserrors.AccessNotFound error is returned if no
	// access can be found for this user, and (only in the case of external users),
	// the everyone@external user.
	ReadUserAccessLevelForTargetAddingMissingUser(ctx context.Context, subject string, target permission.ID) (permission.Access, error)
}

type MacaroonService interface {
	dbrootkeystore.ContextBacking
	BakeryConfigService
}

type BakeryConfigService interface {
	GetLocalUsersKey(context.Context) (*bakery.KeyPair, error)
	GetLocalUsersThirdPartyKey(context.Context) (*bakery.KeyPair, error)
	GetExternalUsersThirdPartyKey(context.Context) (*bakery.KeyPair, error)
}

// NewStateAuthenticatorFunc is a function type satisfied by
// NewStateAuthenticator.
type NewStateAuthenticatorFunc func(
	ctx context.Context,
	statePool *state.StatePool,
	controllerModelUUID string,
	controllerConfigService ControllerConfigService,
	accessService AccessService,
	macaroonService MacaroonService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (macaroon.LocalMacaroonAuthenticator, error)

// NewStateAuthenticator returns a new LocalMacaroonAuthenticator that
// authenticates users and agents using the given state pool. The
// authenticator will register handlers into the mux for dealing with
// local macaroon logins.
func NewStateAuthenticator(
	ctx context.Context,
	statePool *state.StatePool,
	controllerModelUUID string,
	controllerConfigService ControllerConfigService,
	accessService AccessService,
	macaroonService MacaroonService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (macaroon.LocalMacaroonAuthenticator, error) {
	systemState, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	agentAuthFactory := authentication.NewAgentAuthenticatorFactory(systemState, nil)
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(ctx, statePool, controllerModelUUID, controllerConfigService, accessService, macaroonService, agentAuthFactory, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	errH := stateAuthenticator.AddHandlers(mux)
	if errH != nil {
		return nil, errors.Trace(errH)
	}
	go stateAuthenticator.Maintain(abort)
	return stateAuthenticator, nil
}
