// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/permission/state"
)

// State describes retrieval and persistence methods for user permission on
// various targets.
type State interface {
	// CreatePermission gives the user access per the provided spec.
	// It requires the user/target combination has not already been
	// created.
	CreatePermission(ctx context.Context, spec state.UserAccessSpec) (permission.UserAccess, error)

	// DeletePermission removes the given subject's (user) access to the
	// given target.
	DeletePermission(ctx context.Context, subject string, target permission.ID) error

	// UpsertPermission updates the permission on the target for the given
	// subject (user). The api user must have Admin permission on the target. If a
	// subject does not exist, it is created using the subject and api user. Access
	// can be granted or revoked.
	UpsertPermission(ctx context.Context, args state.UpsertPermissionArgs) error

	// ReadUserAccessForTarget returns the subject's (user) access for the
	// given user on the given target.
	ReadUserAccessForTarget(ctx context.Context, subject string, target permission.ID) (permission.UserAccess, error)

	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target permission.ID) (permission.Access, error)

	// ReadAllUserAccessForUser returns a slice of the user access the given
	// subject's (user) has for any access type.
	ReadAllUserAccessForUser(ctx context.Context, subject string) ([]permission.UserAccess, error)

	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	ReadAllUserAccessForTarget(ctx context.Context, target permission.ID) ([]permission.UserAccess, error)

	// ReadAllAccessTypeForUser return a slice of user access for the subject
	// (user) specified and of the given access type.
	// E.G. All clouds the user has access to.
	ReadAllAccessTypeForUser(ctx context.Context, subject string, accessType permission.ObjectType) ([]permission.UserAccess, error)
}

// UserAccessSpec defines the attributes that can be set when adding a new
// user access.
type UserAccessSpec struct {
	User   string
	Target permission.ID
	Access permission.Access
}

// Validate validates that the access and target specified in the
// spec are values allowed together and that the User is not an
// empty string. If any of these are untrue, a NotValid error is
// returned.
func (u UserAccessSpec) validate() error {
	if u.User == "" {
		return errors.NotValidf("empty user")
	}
	if err := u.Target.Validate(); err != nil {
		return err
	}
	if err := u.Target.ValidateAccess(u.Access); err != nil {
		return err
	}
	return nil
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
func (s *Service) CreatePermission(ctx context.Context, spec UserAccessSpec) (permission.UserAccess, error) {
	if err := spec.validate(); err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	userAccess, err := s.st.CreatePermission(ctx, state.UserAccessSpec{
		User:   spec.User,
		Target: permission.ID{ObjectType: spec.Target.ObjectType, Key: spec.Target.Key},
		Access: spec.Access,
	})
	return userAccess, errors.Trace(err)
}

// DeletePermission removes the given user's access to the given target.
// A NotValid error is returned if the subject (user) string is empty, or
// the target is not valid. Any errors from the state layer are passed through.
func (s *Service) DeletePermission(ctx context.Context, subject string, target permission.ID) error {
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
func (s *Service) ReadUserAccessForTarget(ctx context.Context, subject string, target permission.ID) (permission.UserAccess, error) {
	if subject == "" {
		return permission.UserAccess{}, errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := target.Validate(); err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	userAccess, err := s.st.ReadUserAccessForTarget(ctx, subject, target)
	return userAccess, errors.Trace(err)
}

// ReadUserAccessLevelForTarget returns the user access level for the
// given user on the given target. A NotValid error is returned if the
// subject (user) string is empty, or the target is not valid. Any errors
// from the state layer are passed through.
func (s *Service) ReadUserAccessLevelForTarget(ctx context.Context, subject string, target permission.ID) (permission.Access, error) {
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
func (s *Service) ReadAllUserAccessForTarget(ctx context.Context, target permission.ID) ([]permission.UserAccess, error) {
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
func (s *Service) ReadAllUserAccessForUser(ctx context.Context, subject string) ([]permission.UserAccess, error) {
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
func (s *Service) ReadAllAccessTypeForUser(ctx context.Context, subject string, accessType permission.ObjectType) ([]permission.UserAccess, error) {
	if subject == "" {
		return nil, errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := accessType.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	userAccess, err := s.st.ReadAllAccessTypeForUser(ctx, subject, accessType)
	return userAccess, errors.Trace(err)
}

type AccessChange string

const (
	Grant  AccessChange = "grant"
	Revoke AccessChange = "revoke"
)

// UpsertPermissionArgs are necessary arguments to run
// UpdatePermissionOnTarget.
type UpsertPermissionArgs struct {
	// Access is what the permission access should change to.
	Access permission.Access
	// AddUser will add the subject if the user does not exist.
	AddUser bool
	// ApiUser is the user requesting the change, they must have
	// permission to do it as well.
	ApiUser string
	// What type of change to access is needed, grant or revoke?
	Change AccessChange
	// Subject is the subject of the permission, e.g. user.
	Subject string
	// Target is the thing the subject's permission to is being
	// updated on.
	Target permission.ID
}

func (args UpsertPermissionArgs) validate() error {
	if args.ApiUser == "" {
		return errors.Trace(errors.NotValidf("empty api user"))
	}
	if args.Subject == "" {
		return errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := args.Target.ValidateAccess(args.Access); err != nil {
		return errors.Trace(err)
	}
	if args.Change != Grant && args.Change != Revoke {
		return errors.Trace(errors.NotValidf("change %q", args.Change))
	}
	return nil
}

// UpsertPermission updates the permission on the target for the given
// subject (user). The api user must have Admin permission on the target. If a
// subject does not exist, it is created using the subject and api user. Access
// can be granted or revoked. Revoking the permission on a user which does not
// exist, is a no-op, AddUser will be ignored.
func (s *Service) UpsertPermission(ctx context.Context, args UpsertPermissionArgs) error {
	if err := args.validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(s.st.UpsertPermission(ctx, state.UpsertPermissionArgs{}))
}
