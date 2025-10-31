// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	userservice "github.com/juju/juju/domain/access/service"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/proxy"
)

// ControllerConfigService defines the methods required to get the controller
// configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)

	// WatchControllerConfig watches the controller config for changes.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// ProxyService defines the methods required to get proxy details.
type ProxyService interface {
	// GetConnectionProxyInfo returns the proxy information for the controller.
	GetConnectionProxyInfo(context.Context) (proxy.Proxier, error)
}

// UserService defines the methods required to get user details.
type UserService interface {
	// GetUserByName returns the user with the given name.
	GetUserByName(context.Context, user.Name) (user.User, error)
	// SetPasswordWithActivationKey will use the activation key from the user
	// to then apply the payload password.
	SetPasswordWithActivationKey(ctx context.Context, name user.Name, nonce, box []byte) (userservice.Sealer, error)
}

// MacaroonService defines the method required to manage macaroons.
type MacaroonService interface {
	dbrootkeystore.ContextBacking
	BakeryConfigService
}

// BakeryConfigService manages macaroon bakery config storage.
type BakeryConfigService interface {
	// GetOffersThirdPartyKey returns the key pair used with the cross-model
	// offers bakery.
	GetOffersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
	// GetExternalUsersThirdPartyKey returns the third party key pair used with
	// the external users bakery.
	GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
}

// ModelInfoService provides access to information about the current model.
type ModelInfoService interface {
	// GetModelInfo returns the information for the current model.
	GetModelInfo(ctx context.Context) (model.ModelInfo, error)
}

// AccessService provides information about users and permissions.
type AccessService interface {
	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	// If the access level of a user cannot be found then
	// accesserrors.AccessNotFound is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)
}

// StatusService provides access to the status service.
type StatusService interface {
	// GetOfferStatus returns the status of the specified offer. This status shadows
	// the status of the application that the offer belongs to, except in the case
	// where the application or offer has been removed. Then a Terminated status is
	// returned.
	GetOfferStatus(context.Context, offer.UUID) (status.StatusInfo, error)
}

// RelationService provides access to the relation service.
type RelationService interface {
	// GetRelationUnits returns the current state of the relation units.
	GetFullRelationUnitChange(context.Context, relation.UUID, application.UUID) (domainrelation.FullRelationUnitChange, error)

	// GetInScopeUnits returns the units of an application that are in scope for the
	// given relation.
	GetInScopeUnits(context.Context, application.UUID, relation.UUID) ([]unit.Name, error)

	// GetRelationLifeSuspendedStatus returns a life/suspended status
	// struct for a specified relation uuid.
	GetRelationLifeSuspendedStatus(
		ctx context.Context,
		relationUUID relation.UUID,
	) (domainrelation.RelationLifeSuspendedStatus, error)

	// GetUnitSettingsForUnits returns the settings for the given units.
	GetUnitSettingsForUnits(context.Context, relation.UUID, []unit.Name) ([]domainrelation.UnitSettings, error)

	// GetRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	GetRelationApplicationSettings(context.Context, relation.UUID, application.UUID) (map[string]string, error)
}
