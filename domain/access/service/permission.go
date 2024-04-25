// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/internal/uuid"
)

// PermissionService provides the API for working with permissions.
type PermissionService struct {
	st PermissionState
}

// NewPermissionService returns a new PermissionService for interacting with the underlying
// permission state.
func NewPermissionService(st PermissionState) *PermissionService {
	return &PermissionService{
		st: st,
	}
}

// CreatePermission gives the user access per the provided spec. All errors
// are passed through from the spec validation and state layer.
func (s *PermissionService) CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error) {
	if err := spec.Validate(); err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}
	userAccess, err := s.st.CreatePermission(ctx, newUUID, spec)
	return userAccess, errors.Trace(err)
}

// DeletePermission removes the given user's access to the given target.
// A NotValid error is returned if the subject (user) string is empty, or
// the target is not valid. Any errors from the state layer are passed through.
func (s *PermissionService) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
	if subject == "" {
		return errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := target.Validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(s.st.DeletePermission(ctx, subject, target))
}

// ReadUserAccessForTarget returns the user access for the given user on
// the given target. A NotValid error is returned if the subject (user)
// string is empty, or the target is not valid. Any errors from the state
// layer are passed through.
func (s *PermissionService) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
	if subject == "" {
		return corepermission.UserAccess{}, errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := target.Validate(); err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}
	userAccess, err := s.st.ReadUserAccessForTarget(ctx, subject, target)
	return userAccess, errors.Trace(err)
}

// ReadUserAccessLevelForTarget returns the user access level for the
// given user on the given target. A NotValid error is returned if the
// subject (user) string is empty, or the target is not valid. Any errors
// from the state layer are passed through.
func (s *PermissionService) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
	if subject == "" {
		return "", errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := target.Validate(); err != nil {
		return "", errors.Trace(err)
	}
	access, err := s.st.ReadUserAccessLevelForTarget(ctx, subject, target)
	return access, errors.Trace(err)
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target. A NotValid error is returned if the
// target is not valid. Any errors from the state layer are passed through.
func (s *PermissionService) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	if err := target.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	userAccess, err := s.st.ReadAllUserAccessForTarget(ctx, target)
	return userAccess, errors.Trace(err)
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// user has for any access type. // A NotValid error is returned if the
// subject (user) string is empty. Any errors from the state layer are
// passed through.
func (s *PermissionService) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	if subject == "" {
		return nil, errors.Trace(errors.NotValidf("empty subject"))
	}
	userAccess, err := s.st.ReadAllUserAccessForUser(ctx, subject)
	return userAccess, errors.Trace(err)
}

// ReadAllAccessForUserAndObjectType returns a slice of user access for the
// user specified and of the given object type.
// A NotValid error is returned if the given access type does not exist,
// or the subject (user) is an empty string.
// E.G. All clouds the user has access to.
func (s *PermissionService) ReadAllAccessForUserAndObjectType(ctx context.Context, subject string, objectType corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	if subject == "" {
		return nil, errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := objectType.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	userAccess, err := s.st.ReadAllAccessForUserAndObjectType(ctx, subject, objectType)
	return userAccess, errors.Trace(err)
}

// UpdatePermission updates the permission on the target for the given
// subject (user). The api user must have Superuser access or Admin access
// on the target. If a subject does not exist and the args specify, it is
// created using the subject and api user. Adding the user would typically
// only happen for updates to model access. Access can be granted or revoked.
// Revoking Read access will delete the permission.
func (s *PermissionService) UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error {
	if err := args.Validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(s.st.UpsertPermission(ctx, args))
}
