// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/permission"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for user permission on
// various targets.
type State interface {
	// CreatePermission gives the user access per the provided spec.
	// It requires the user/target combination has not already been
	// created.
	CreatePermission(ctx context.Context, uuid uuid.UUID, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)

	// DeletePermission removes the given subject's (user) access to the
	// given target.
	DeletePermission(ctx context.Context, subject string, target corepermission.ID) error

	// UpsertPermission updates the permission on the target for the given
	// subject (user). The api user must have Admin permission on the target. If a
	// subject does not exist, it is created using the subject and api user. Access
	// can be granted or revoked.
	UpsertPermission(ctx context.Context, args permission.UpsertPermissionArgs) error

	// ReadUserAccessForTarget returns the subject's (user) access for the
	// given user on the given target.
	ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error)

	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error)

	// ReadAllUserAccessForUser returns a slice of the user access the given
	// subject's (user) has for any access type.
	ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error)

	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error)

	// ReadAllAccessTypeForUser return a slice of user access for the subject
	// (user) specified and of the given access type.
	// E.G. All clouds the user has access to.
	ReadAllAccessTypeForUser(ctx context.Context, subject string, accessType corepermission.ObjectType) ([]corepermission.UserAccess, error)
}

// Service provides the API for working with permissions.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying
// permission state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// CreatePermission gives the user access per the provided spec. All errors
// are passed through from the spec validation and state layer.
func (s *Service) CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error) {
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
func (s *Service) DeletePermission(ctx context.Context, subject string, target corepermission.ID) error {
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
func (s *Service) ReadUserAccessForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.UserAccess, error) {
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
func (s *Service) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error) {
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
func (s *Service) ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error) {
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
func (s *Service) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]corepermission.UserAccess, error) {
	if subject == "" {
		return nil, errors.Trace(errors.NotValidf("empty subject"))
	}
	userAccess, err := s.st.ReadAllUserAccessForUser(ctx, subject)
	return userAccess, errors.Trace(err)
}

// ReadAllAccessTypeForUser return a slice of user access for the user
// specified and of the given access type. A NotValid error is returned if
// the given access type does not exist, or the subject (user) is an empty
// string.
// E.G. All clouds the user has access to.
func (s *Service) ReadAllAccessTypeForUser(ctx context.Context, subject string, accessType corepermission.ObjectType) ([]corepermission.UserAccess, error) {
	if subject == "" {
		return nil, errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := accessType.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	userAccess, err := s.st.ReadAllAccessTypeForUser(ctx, subject, accessType)
	return userAccess, errors.Trace(err)
}

// UpsertPermission updates the permission on the target for the given
// subject (user). The api user must have Admin permission on the target. If a
// subject does not exist, it is created using the subject and api user. Access
// can be granted or revoked. Revoking the permission on a user which does not
// exist, is a no-op, AddUser will be ignored.
func (s *Service) UpsertPermission(ctx context.Context, args permission.UpsertPermissionArgs) error {
	if err := args.Validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(s.st.UpsertPermission(ctx, args))
}
