// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainapplication "github.com/juju/juju/domain/application"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/domain/relation"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/environs/config"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// AddRelationNetworkIngress adds ingress network CIDRs for the specified
	// relation.
	// The CIDRs are added to the relation_network_ingress table.
	AddRelationNetworkIngress(ctx context.Context, relationUUID corerelation.UUID, saasIngressAllow []string, cidrs []string) error

	// AddConsumedRelation adds a new synthetic application representing
	// the application on the consuming model, to this, the offering model.
	// The synthetic application is used to create a relation with the
	// provided charm.Relation from the consuming side and the offering
	// application endpoint name in the current model.
	AddConsumedRelation(ctx context.Context, args crossmodelrelationservice.AddConsumedRelationArgs) error

	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID offer.UUID) (string, coreapplication.UUID, error)

	// GetOfferingApplicationToken returns the offering application token (uuid)
	// for the given relation UUID.
	GetOfferingApplicationToken(ctx context.Context, relationUUID corerelation.UUID) (coreapplication.UUID, error)

	// GetOfferUUIDByRelationUUID returns the offer UUID corresponding to
	// the cross model relation UUID.
	GetOfferUUIDByRelationUUID(ctx context.Context, relationUUID corerelation.UUID) (offer.UUID, error)

	// EnsureUnitsExist ensures that the given synthetic units exist in the local
	// model.
	EnsureUnitsExist(ctx context.Context, appUUID coreapplication.UUID, units []unit.Name) error

	// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any
	// unit of the specified app and returns a watcher which notifies of secret URIs
	// that have had a new revision added.
	WatchRemoteConsumedSecretsChanges(ctx context.Context, appUUID coreapplication.UUID) (watcher.StringsWatcher, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// SecretService provides access to secrets.
type SecretService interface {
	// GetLatestRevisions returns the latest secret revisions for the specified URIs.
	GetLatestRevisions(ctx context.Context, uris []*coresecrets.URI) (map[string]int, error)
}

// StatusService provides access to the status service.
type StatusService interface {
	// GetOfferStatus returns the status of the specified offer. This status
	// shadows the status of the application that the offer belongs to, except
	// in the case where the application or offer has been removed. Then a
	// Terminated status is returned.
	GetOfferStatus(context.Context, offer.UUID) (status.StatusInfo, error)

	// WatchOfferStatus watches the changes to the derived display status of
	// the specified application.
	WatchOfferStatus(context.Context, offer.UUID) (watcher.NotifyWatcher, error)

	// SetRemoteRelationStatus sets the status of the relation to the status
	// provided.
	SetRemoteRelationStatus(ctx context.Context, relationUUID corerelation.UUID, statusInfo status.StatusInfo) error
}

// RelationService provides access to relations.
type RelationService interface {
	// GetConsumerRelationUnitsChange returns the versions of the relation units
	// settings and any departed units.
	GetConsumerRelationUnitsChange(
		context.Context,
		corerelation.UUID,
		coreapplication.UUID,
	) (relation.ConsumerRelationUnitsChange, error)

	// GetRelationDetails returns relation details for the given relationUUID.
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (domainrelation.RelationDetails, error)

	// GetRelationKeyByUUID returns the relation key for the given UUID.
	GetRelationKeyByUUID(ctx context.Context, relationUUID string) (corerelation.Key, error)

	// GetRelationUnitUUID returns the relation unit UUID for the given unit for
	// the given relation.
	GetRelationUnitUUID(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
	) (corerelation.UnitUUID, error)

	// SetRelationRemoteApplicationAndUnitSettings will set the application and
	// unit settings for a remote relation. If the unit has not yet entered
	// scope, it will force the unit to enter scope. All settings will be
	// replaced with the provided settings.
	// This will ensure that the application, relation and units exist and that
	// they are alive.
	SetRelationRemoteApplicationAndUnitSettings(
		ctx context.Context,
		applicationUUID coreapplication.UUID,
		relationUUID corerelation.UUID,
		applicationSettings map[string]string,
		unitSettings map[unit.Name]map[string]string,
	) error

	// SetRemoteRelationSuspendedState sets the suspended state of the specified
	// remote relation in the local model. The relation must be a cross-model
	// relation.
	SetRemoteRelationSuspendedState(ctx context.Context, relationUUID corerelation.UUID, suspended bool, reason string) error

	// WatchRelationUnits returns a watcher for changes to the units
	// in the given relation in the local model.
	WatchRelationUnits(context.Context, corerelation.UUID, coreapplication.UUID) (watcher.NotifyWatcher, error)
}

// ApplicationService provides access to applications.
type ApplicationService interface {
	// GetApplicationDetails returns application details for the given appUUID.
	// This includes the life status and the name of the application.
	GetApplicationDetails(ctx context.Context, appUUID coreapplication.UUID) (domainapplication.ApplicationDetails, error)
}

// RemovalService provides the ability to remove remote relations.
type RemovalService interface {
	// RemoveRelation checks if a relation with the input UUID exists.
	// If it does, the relation is guaranteed after this call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	RemoveRemoteRelation(
		ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration,
	) (removal.UUID, error)

	// LeaveScope updates the relation to indicate that the unit represented by
	// the input relation unit UUID is not in the implied relation scope.
	LeaveScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) error
}
