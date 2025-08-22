// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/uuid"
)

// AccessService defines the interface for interacting with the access domain.
type AccessService interface {
	// CreatePermission gives the user access per the provided spec. All errors
	// are passed through from the spec validation and state layer.
	CreatePermission(
		ctx context.Context,
		spec corepermission.UserAccessSpec,
	) (corepermission.UserAccess, error)

	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)

	// UpdatePermission updates the permission on the target for the given subject
	// (user). If the subject is an external user, and they do not exist, they are
	// created. Access can be granted or revoked. Revoking Read access will delete
	// the permission.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
}

// ModelService defines the interface for interacting with the model domain.
type ModelService interface {
	// GetModelByNameAndQualifier returns the model associated with the given model name and qualifier.
	// The following errors may be returned:
	// - [modelerrors.NotFound] if no model exists.
	// - [github.com/juju/juju/core/errors.NotValid] if qualifier is not valid.
	GetModelByNameAndQualifier(
		ctx context.Context,
		name string,
		qualifier coremodel.Qualifier,
	) (coremodel.Model, error)
}

// CrossModelRelationService defines the interface for interacting with the crossmodelrelation domain.
type CrossModelRelationService interface {
	// GetOfferUUID returns the uuid for the provided offer URL.
	GetOfferUUID(ctx context.Context, offerURL *crossmodel.OfferURL) (uuid.UUID, error)

	// GetOffers returns offer details for all offers satisfying any of the
	// provided filters.
	GetOffers(
		ctx context.Context,
		filters []crossmodelrelation.OfferFilter,
	) ([]*crossmodelrelation.OfferDetail, error)

	// Offer updates an existing offer, or creates a new offer if it does not exist.
	// Permissions are created for a new offer only.
	Offer(
		ctx context.Context,
		args crossmodelrelation.ApplicationOfferArgs,
	) error
}

// RemovalService defines operations for removing juju entities,
// such as offers.
type RemovalService interface {
	// RemoveOffer removes the offer from the model.
	RemoveOffer(ctx context.Context, offerUUID uuid.UUID, force bool) error
}
