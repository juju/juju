// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/uuid"
)

// State represents a type for interacting with the underlying state.
type State interface {
	UserState
	PermissionState
}

// UserState describes retrieval and persistence methods for user identify and
// authentication.
type UserState interface {
	// AddUser will add a new user to the database. If the user already exists
	// an error that satisfies accesserrors.UserAlreadyExists will be returned.
	// If the users creator is set and does not exist then an error that satisfies
	// accesserrors.UserCreatorUUIDNotFound will be returned.
	AddUser(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.AccessSpec,
	) error

	// AddUserWithPasswordHash will add a new user to the database with the
	// provided password hash and salt. If the user already exists an error that
	// satisfies accesserrors.UserAlreadyExists will be returned. If the users creator
	// does not exist or has been previously removed an error that satisfies
	// accesserrors.UserCreatorUUIDNotFound will be returned.
	AddUserWithPasswordHash(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.AccessSpec,
		passwordHash string,
		passwordSalt []byte,
	) error

	// AddUserWithActivationKey will add a new user to the database with the
	// provided activation key. If the user already exists an error that
	// satisfies accesserrors.UserAlreadyExists will be returned. if the users creator
	// does not exist or has been previously removed an error that satisfies
	// accesserrors.UserCreatorUUIDNotFound will be returned.
	AddUserWithActivationKey(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.AccessSpec,
		activationKey []byte,
	) error

	// GetAllUsers will retrieve all users with authentication information
	// (last login, disabled) from the database. If no users exist an empty slice
	// will be returned.
	GetAllUsers(context.Context) ([]user.User, error)

	// GetUser will retrieve the user with authentication information (last login, disabled)
	// specified by UUID from the database. If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUser(context.Context, user.UUID) (user.User, error)

	// GetUserByName will retrieve the user with authentication information (last login, disabled)
	// specified by name from the database. If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUserByName(ctx context.Context, name string) (user.User, error)

	// GetUserByAuth will retrieve the user with checking authentication information
	// specified by name and password from the database. If the user does not exist
	// an error that satisfies accesserrors.UserNotFound will be returned.
	GetUserByAuth(context.Context, string, auth.Password) (user.User, error)

	// RemoveUser marks the user as removed. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	// RemoveUser will also remove any credentials and activation codes for the
	// user. If no user exists for the given user name then an error that satisfies
	// accesserrors.UserNotFound will be returned.
	RemoveUser(context.Context, string) error

	// SetActivationKey removes any active passwords for the user and sets the
	// activation key. If no user is found for the supplied user name an error
	// is returned that satisfies accesserrors.UserNotFound.
	SetActivationKey(context.Context, string, []byte) error

	// GetActivationKey will retrieve the activation key for the user.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	GetActivationKey(context.Context, string) ([]byte, error)

	// SetPasswordHash removes any active activation keys and sets the user
	// password hash and salt. If no user is found for the supplied user name an error
	// is returned that satisfies accesserrors.UserNotFound.
	SetPasswordHash(context.Context, string, string, []byte) error

	// EnableUserAuthentication will enable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	EnableUserAuthentication(context.Context, string) error

	// DisableUserAuthentication will disable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	DisableUserAuthentication(context.Context, string) error

	// UpdateLastLogin will update the last login time for the user.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	UpdateLastLogin(context.Context, string) error
}

// PermissionState describes retrieval and persistence methods for user
// permission on various targets.
type PermissionState interface {
	// CreatePermission gives the user access per the provided spec.
	// It requires the user/target combination has not already been
	// created.
	CreatePermission(ctx context.Context, uuid uuid.UUID, spec permission.UserAccessSpec) (permission.UserAccess, error)

	// DeletePermission removes the given subject's (user) access to the
	// given target.
	DeletePermission(ctx context.Context, subject string, target permission.ID) error

	// UpsertPermission updates the permission on the target for the given
	// subject (user). The api user must have Admin permission on the target. If a
	// subject does not exist, it is created using the subject and api user. Access
	// can be granted or revoked.
	UpsertPermission(ctx context.Context, args access.UpsertPermissionArgs) error

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

// Service provides the API for working with users.
type Service struct {
	*UserService
	*PermissionService
}

// NewService returns a new Service for interacting with the underlying user
// state.
func NewService(st State) *Service {
	return &Service{
		UserService: NewUserService(st),
	}
}
