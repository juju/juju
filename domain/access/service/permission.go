// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/errors"
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := spec.Validate(); err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}
	userAccess, err := s.st.CreatePermission(ctx, newUUID, spec)
	return userAccess, errors.Capture(err)
}

// DeletePermission removes the given user's access to the given target.
// A NotValid error is returned if the subject (user) string is empty, or
// the target is not valid. Any errors from the state layer are passed through.
func (s *PermissionService) DeletePermission(ctx context.Context, subject user.Name, target corepermission.ID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if subject.IsZero() {
		return errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := target.Validate(); err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(s.st.DeletePermission(ctx, subject, target))
}

// ReadUserAccessForTarget returns the user access for the given user on
// the given target. A NotValid error is returned if the subject (user)
// string is empty, or the target is not valid. Any errors from the state
// layer are passed through.
func (s *PermissionService) ReadUserAccessForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.UserAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if subject.IsZero() {
		return corepermission.UserAccess{}, errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := target.Validate(); err != nil {
		return corepermission.UserAccess{}, errors.Capture(err)
	}
	userAccess, err := s.st.ReadUserAccessForTarget(ctx, subject, target)
	return userAccess, errors.Capture(err)
}

// EnsureExternalUserIfAuthorized checks if an external user is missing from the
// database and has permissions on an object. If they do then they will be
// added. This ensures that juju has a record of external users that have
// inherited their permissions from everyone@external.
func (s *PermissionService) EnsureExternalUserIfAuthorized(ctx context.Context, subject user.Name, target corepermission.ID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if subject.IsZero() {
		return errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := target.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := s.st.EnsureExternalUserIfAuthorized(ctx, subject, target); errors.Is(err, accesserrors.UserNotFound) {
		return errors.Capture(err)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// ReadUserAccessLevelForTarget returns the user access level for the
// given user on the given target. A NotValid error is returned if the
// subject (user) string is empty, or the target is not valid. Any errors
// from the state layer are passed through.
// If the access level of a user cannot be found then
// [accesserrors.AccessNotFound] is returned.
func (s *PermissionService) ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target corepermission.ID) (corepermission.Access, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if subject.IsZero() {
		return "", errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := target.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	access, err := s.st.ReadUserAccessLevelForTarget(ctx, subject, target)
	return access, errors.Capture(err)
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target. A NotValid error is returned if the
// target is not valid. Any errors from the state layer are passed through.
func (s *PermissionService) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := target.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	userAccess, err := s.st.ReadAllUserAccessForTarget(ctx, target)
	return userAccess, errors.Capture(err)
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// user has for any access type. // A NotValid error is returned if the
// subject (user) string is empty. Any errors from the state layer are
// passed through.
func (s *PermissionService) ReadAllUserAccessForUser(ctx context.Context, subject user.Name) ([]corepermission.UserAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if subject.IsZero() {
		return nil, errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	userAccess, err := s.st.ReadAllUserAccessForUser(ctx, subject)
	return userAccess, errors.Capture(err)
}

// ReadAllAccessForUserAndObjectType returns a slice of user access for the
// user specified and of the given object type.
// A NotValid error is returned if the given access type does not exist,
// or the subject (user) is an empty string.
// E.G. All clouds the user has access to.
func (s *PermissionService) ReadAllAccessForUserAndObjectType(ctx context.Context, subject user.Name, objectType corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if subject.IsZero() {
		return nil, errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := objectType.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	userAccess, err := s.st.ReadAllAccessForUserAndObjectType(ctx, subject, objectType)
	return userAccess, errors.Capture(err)
}

// UpdatePermission updates the permission on the target for the given subject
// (user). If the subject is an external user, and they do not exist, they are
// created. Access can be granted or revoked. Revoking Read access will delete
// the permission.
// [accesserrors.UserNotFound] is returned if the user is local and does not
// exist in the users table.
// [accesserrors.PermissionAccessGreater] is returned if the user is being
// granted an access level greater or equal to what they already have.
func (s *PermissionService) UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := args.Validate(); err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(s.st.UpdatePermission(ctx, args))
}

// AllModelAccessForCloudCredential for a given (cloud) credential key, return all
// model name and model access level combinations.
func (s *PermissionService) AllModelAccessForCloudCredential(ctx context.Context, key credential.Key) ([]access.CredentialOwnerModelAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	results, err := s.st.AllModelAccessForCloudCredential(ctx, key)
	return results, errors.Capture(err)
}
