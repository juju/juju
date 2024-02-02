// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/permission"
)

// State describes retrieval and persistence methods for user permission on
// various targets.
type State interface {
	// CreatePermission gives the user access per the provided spec.
	CreatePermission(ctx context.Context, spec UserAccessSpec) (permission.UserAccess, error)

	// DeletePermission removes the given user's access to the given target.
	DeletePermission(ctx context.Context, subject names.UserTag, target names.Tag) error

	// UpdatePermission updates the user's access to the given target to the
	// given access.
	UpdatePermission(ctx context.Context, subject names.UserTag, target names.Tag, access permission.Access) error

	// ReadUserAccessForTarget returns the user access for the given user on
	// the given target.
	ReadUserAccessForTarget(ctx context.Context, subject names.UserTag, target names.Tag) (permission.UserAccess, error)

	// ReadUserAccessLevelForTarget returns the user access level for the
	// given user on the given target.
	ReadUserAccessLevelForTarget(ctx context.Context, subject names.UserTag, target names.Tag) (permission.Access, error)

	// ReadAllUserAccessForUser returns a slice of the user access the given
	// user has for any access type.
	ReadAllUserAccessForUser(ctx context.Context, subject names.UserTag) ([]permission.UserAccess, error)

	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	ReadAllUserAccessForTarget(ctx context.Context, target names.Tag) ([]permission.UserAccess, error)

	// ReadAllAccessTypeForUser return a slice of user access for the user
	// specified and of the given access type.
	// E.G. All clouds the user has access to.
	ReadAllAccessTypeForUser(ctx context.Context, subject names.UserTag, accessType permission.AccessType) ([]permission.UserAccess, error)
}

// UserAccessSpec defines the attributes that can be set when adding a new
// user access.
type UserAccessSpec struct {
	User       names.UserTag
	Target     names.Tag
	Access     permission.Access
	AccessType permission.AccessType
}

// Validate validates that the access and access type specified in the
// spec are values allowed.
func (u UserAccessSpec) validate() error {
	if err := validateTarget(u.Target); err != nil {
		return err
	}
	if err := permission.ValidateAccessForAccessType(u.Access, u.AccessType); err != nil {
		return err
	}
	return nil
}

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

// CreatePermission gives the user access per the provided spec. An error is
// returned if the access and access type of the spec are not valid.
func (s *Service) CreatePermission(ctx context.Context, spec UserAccessSpec) (permission.UserAccess, error) {
	if err := spec.validate(); err != nil {
		return permission.UserAccess{}, err
	}
	return s.st.CreatePermission(ctx, spec)
}

// DeletePermission removes the given user's access to the given target.
// An error is returned if the given target's access type does not exist.
func (s *Service) DeletePermission(ctx context.Context, subject names.UserTag, target names.Tag) error {
	if err := validateTarget(target); err != nil {
		return err
	}
	return s.st.DeletePermission(ctx, subject, target)
}

// ReadUserAccessForTarget returns the user access for the given user on
// the given target. An error is returned if the given target's access
// type does not exist.
func (s *Service) ReadUserAccessForTarget(ctx context.Context, subject names.UserTag, target names.Tag) (permission.UserAccess, error) {
	if err := validateTarget(target); err != nil {
		return permission.UserAccess{}, err
	}
	return s.st.ReadUserAccessForTarget(ctx, subject, target)
}

// ReadUserAccessLevelForTarget returns the user access level for the
// given user on the given target. An error is returned if the given
// target's access type does not exist.
func (s *Service) ReadUserAccessLevelForTarget(ctx context.Context, subject names.UserTag, target names.Tag) (permission.Access, error) {
	if err := validateTarget(target); err != nil {
		return "", err
	}
	return s.st.ReadUserAccessLevelForTarget(ctx, subject, target)
}

// ReadAllUserAccessForTarget return a slice of user access for all users
// with access to the given target. An error is returned if the given
// target's access type does not exist.
func (s *Service) ReadAllUserAccessForTarget(ctx context.Context, target names.Tag) ([]permission.UserAccess, error) {
	if err := validateTarget(target); err != nil {
		return nil, err
	}
	return s.st.ReadAllUserAccessForTarget(ctx, target)
}

// ReadAllUserAccessForUser returns a slice of the user access the given
// user has for any access type.
func (s *Service) ReadAllUserAccessForUser(ctx context.Context, subject names.UserTag) ([]permission.UserAccess, error) {
	return s.st.ReadAllUserAccessForUser(ctx, subject)
}

// ReadAllAccessTypeForUser return a slice of user access for the user
// specified and of the given access type. An error is returned if the
// given access type does not exist.
// E.G. All clouds the user has access to.
func (s *Service) ReadAllAccessTypeForUser(ctx context.Context, subject names.UserTag, accessType permission.AccessType) ([]permission.UserAccess, error) {
	if err := accessType.Validate(); err != nil {
		return nil, err
	}
	return s.st.ReadAllAccessTypeForUser(ctx, subject, accessType)
}

// UpdatePermission updates the user's access to the given target to the
// given access.
func (s *Service) UpdatePermission(ctx context.Context, subject names.UserTag, target names.Tag, access permission.Access) error {
	var accessType permission.AccessType
	// Unfortunately the tag kind values are not an exact
	// match for permission access type values. Convert here.
	switch target.Kind() {
	case names.CloudTagKind:
		accessType = permission.Cloud
	case names.ControllerTagKind:
		accessType = permission.Controller
	case names.ModelTagKind:
		accessType = permission.Model
	case names.ApplicationOfferTagKind:
		accessType = permission.Offer
	default:
		return errors.NotValidf("target tag type %s", target.Kind())
	}
	if err := permission.ValidateAccessForAccessType(access, accessType); err != nil {
		return err
	}
	return s.st.UpdatePermission(ctx, subject, target, access)
}

func validateTarget(target names.Tag) error {
	switch target.Kind() {
	case names.CloudTagKind, names.ControllerTagKind, names.ModelTagKind, names.ApplicationOfferTagKind:
	default:
		return errors.NotValidf("target tag type %q", target.Kind())
	}
	return nil
}
