// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/apiserver/facade"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/servicefactory"
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
	// GetCharmIDByApplicationName returns a charm ID by name. It returns an
	// error if the charm can not be found by the name. This can also be used as
	// a cheap way to see if a charm exists without needing to load the charm
	// metadata.
	GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error)

	// GetCharmMetadataDescription returns the description for the charm using
	// the charm ID.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmMetadataDescription(ctx context.Context, id corecharm.ID) (string, error)
}

// ModelServiceFactory is an interface that provides a way to get model
// scoped services.
type ModelServiceFactory interface {
	Application() ApplicationService
}

// ModelServiceFactoryGetter is an interface that provides a way to get a
// ModelServiceFactory based on a model UUID.
type ModelServiceFactoryGetter interface {
	ServiceFactoryForModel(modelUUID model.UUID) ModelServiceFactory
}

type modelServiceFactoryGetter struct {
	facadeContext facade.MultiModelContext
}

func newModelServiceFactoryGetter(facadeContext facade.MultiModelContext) ModelServiceFactoryGetter {
	return &modelServiceFactoryGetter{
		facadeContext: facadeContext,
	}
}

func (f *modelServiceFactoryGetter) ServiceFactoryForModel(modelUUID model.UUID) ModelServiceFactory {
	return &modelServiceFactory{serviceFactory: f.facadeContext.ServiceFactoryForModel(modelUUID)}
}

type modelServiceFactory struct {
	serviceFactory servicefactory.ServiceFactory
}

func (f *modelServiceFactory) Application() ApplicationService {
	return f.serviceFactory.Application(service.ApplicationServiceParams{})
}
