// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/services"
)

// AccessService provides information about users and permissions.
type AccessService interface {
	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	// If the access level of a user cannot be found then
	// [accesserrors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.Access, error)
	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	// If not user access can be found on the target it will return
	// [accesserrors.PermissionNotFound].
	ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	// CreatePermission gives the user access per the provided spec.
	// If the user provided does not exist or is marked removed,
	// [accesserrors.PermissionNotFound] is returned.
	// If the user provided exists but is marked disabled,
	// [accesserrors.UserAuthenticationDisabled] is returned.
	// If a permission for the user and target key already exists,
	// [accesserrors.PermissionAlreadyExists] is returned.
	CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// UpdatePermission updates the permission on the target for the given subject
	// (user). If the subject is an external user, and they do not exist, they are
	// created. Access can be granted or revoked. Revoking Read access will delete
	// the permission.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// DeletePermission removes the given user's access to the given target.
	// A NotValid error is returned if the subject (user) string is empty, or
	// the target is not valid.
	DeletePermission(ctx context.Context, subject user.Name, target corepermission.ID) error
	// GetUserByName will retrieve the user specified by name from the database.
	// If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)
}

type ApplicationService interface {
	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)

	// GetCharmMetadataDescription returns the description for the charm using the
	// charm name, source and revision.
	GetCharmMetadataDescription(ctx context.Context, locator applicationcharm.CharmLocator) (string, error)
}

// ModelDomainServices is an interface that provides a way to get model
// scoped services.
type ModelDomainServices interface {
	Application() ApplicationService
}

// ModelDomainServicesGetter is an interface that provides a way to get a
// ModelDomainServices based on a model UUID.
type ModelDomainServicesGetter interface {
	DomainServicesForModel(ctx context.Context, modelUUID model.UUID) (ModelDomainServices, error)
}

type modelDomainServicesGetter struct {
	facadeContext facade.MultiModelContext
}

func newModelDomainServicesGetter(facadeContext facade.MultiModelContext) ModelDomainServicesGetter {
	return &modelDomainServicesGetter{
		facadeContext: facadeContext,
	}
}

func (f *modelDomainServicesGetter) DomainServicesForModel(ctx context.Context, modelUUID model.UUID) (ModelDomainServices, error) {
	services, err := f.facadeContext.DomainServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, err
	}
	return &modelDomainServices{domainServices: services}, nil
}

type modelDomainServices struct {
	domainServices services.DomainServices
}

func (f *modelDomainServices) Application() ApplicationService {
	return f.domainServices.Application()
}

type ModelService interface {
	// ListAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	ListAllModels(ctx context.Context) ([]model.Model, error)

	// GetModelByNameAndOwner returns the model associated with the given model name and owner name.
	// The following errors may be returned:
	// - [modelerrors.NotFound] if no model exists
	// - [accesserrors.UserNameNotValid] if ownerName is zero
	GetModelByNameAndOwner(ctx context.Context, name string, ownerName user.Name) (model.Model, error)
}
