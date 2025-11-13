// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/controller"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
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

	// UpdatePermission updates the permission on the target for the given
	// subject (user). If the subject is an external user, and they do not
	// exist, they are created. Access can be granted or revoked. Revoking Read
	// access will delete the permission.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
}

// ModelService defines the interface for interacting with the model domain.
type ModelService interface {
	// ListAllModels returns a slice of all models in the controller. If no models
	// exist an empty slice is returned.
	ListAllModels(ctx context.Context) ([]coremodel.Model, error)

	// GetModelByNameAndQualifier returns the model associated with the given
	// model name and qualifier.
	GetModelByNameAndQualifier(
		ctx context.Context,
		name string,
		qualifier coremodel.Qualifier,
	) (coremodel.Model, error)
}

// CrossModelRelationService defines the interface for interacting with the crossmodelrelation domain.
type CrossModelRelationService interface {
	// GetConsumeDetails returns the offer uuid and endpoints necessary to
	// consume the offer.
	GetConsumeDetails(
		ctx context.Context,
		offerURL crossmodel.OfferURL,
	) (crossmodelrelation.ConsumeDetails, error)

	// GetOfferUUID returns the uuid for the provided offer URL.
	GetOfferUUID(ctx context.Context, offerURL crossmodel.OfferURL) (offer.UUID, error)

	// GetOffersWithConnections returns offer details for all offers satisfying any of the
	// provided filters, including offer connections
	GetOffersWithConnections(
		ctx context.Context,
		filters []crossmodelrelationservice.OfferFilter,
	) ([]*crossmodelrelation.OfferDetailWithConnections, error)

	// Offer updates an existing offer, or creates a new offer if it does not
	// exist. Permissions are created for a new offer only.
	Offer(
		ctx context.Context,
		args crossmodelrelation.ApplicationOfferArgs,
	) error
}

// RemovalService defines operations for removing juju entities,
// such as offers.
type RemovalService interface {
	// RemoveOffer removes the offer from the model.
	RemoveOffer(ctx context.Context, offerUUID offer.UUID, force bool) error
}

// ControllerService defines the interface for interacting with the controller
// domain.
type ControllerService interface {
	// GetControllerInfo returns the controller information.
	GetControllerInfo(ctx context.Context) (controller.ControllerInfo, error)
}
