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
	"github.com/juju/juju/core/unit"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/state"
)

// ControllerConfigService is an interface that can be implemented by
// types that can return a controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// DomainServicesGetter defines methods for getting domain services
// for a model.
type DomainServicesGetter interface {
	// ServicesForModel returns a DomainServices for the given model.
	ServicesForModel(ctx context.Context, modelID coremodel.UUID) (services.DomainServices, error)
}

// AgentPasswordServiceGetter defines the methods required to get a agent
// password service for a model.
type AgentPasswordServiceGetter interface {
	// GetAgentPasswordServiceForModel returns a AgentPasswordService for the
	// given model.
	GetAgentPasswordServiceForModel(ctx context.Context, modelUUID coremodel.UUID) (authentication.AgentPasswordService, error)
}

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// GetUserByAuth returns the user with the given name and password.
	GetUserByAuth(ctx context.Context, name coreuser.Name, password auth.Password) (coreuser.User, error)

	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name coreuser.Name) (coreuser.User, error)

	// UpdateLastModelLogin updates the last login time for the user with the
	// given name on the given model.
	UpdateLastModelLogin(ctx context.Context, name coreuser.Name, modelUUID coremodel.UUID) error

	// EnsureExternalUserIfAuthorized checks if an external user is missing from the
	// database and has permissions on an object. If they do then they will be
	// added. This ensures that juju has a record of external users that have
	// inherited their permissions from everyone@external.
	EnsureExternalUserIfAuthorized(ctx context.Context, subject coreuser.Name, target permission.ID) error

	// ReadUserAccessLevelForTarget returns the user access level for the given
	// user on the given target. A NotValid error is returned if the subject
	// (user) string is empty, or the target is not valid. Any errors from the
	// state layer are passed through. If the access level of a user cannot be
	// found then [accesserrors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject coreuser.Name, target permission.ID) (permission.Access, error)
}

type MacaroonService interface {
	dbrootkeystore.ContextBacking
	BakeryConfigService
}

// PasswordService defines the methods required to set a password hash for a
// unit.
type PasswordService interface {
	// MatchesUnitPasswordHash checks if the password is valid or not.
	MatchesUnitPasswordHash(context.Context, unit.Name, string) (bool, error)
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
	controllerModelUUID coremodel.UUID,
	controllerConfigService ControllerConfigService,
	agentPasswordServiceGetter AgentPasswordServiceGetter,
	accessService AccessService,
	macaroonService MacaroonService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
) (macaroon.LocalMacaroonAuthenticator, error)

// NewStateAuthenticator returns a new LocalMacaroonAuthenticator that
// authenticates users and agents using the given state pool. The
// authenticator will register handlers into the mux for dealing with
// local macaroon logins.
func NewStateAuthenticator(
	ctx context.Context,
	statePool *state.StatePool,
	controllerModelUUID coremodel.UUID,
	controllerConfigService ControllerConfigService,
	agentPasswordServiceGetter AgentPasswordServiceGetter,
	accessService AccessService,
	macaroonService MacaroonService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
) (macaroon.LocalMacaroonAuthenticator, error) {
	systemState, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	passwordService, err := agentPasswordServiceGetter.GetAgentPasswordServiceForModel(ctx, controllerModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	agentAuthGetter := authentication.NewAgentAuthenticatorGetter(passwordService, systemState, nil)
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(
		ctx,
		controllerModelUUID, controllerConfigService,
		agentPasswordServiceGetter,
		accessService,
		macaroonService,
		agentAuthGetter,
		clock,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	errH := stateAuthenticator.AddHandlers(mux)
	if errH != nil {
		return nil, errors.Trace(errH)
	}
	go stateAuthenticator.Maintain(ctx)
	return stateAuthenticator, nil
}
