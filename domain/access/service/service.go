// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
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
		name user.Name,
		displayName string,
		external bool,
		creatorUUID user.UUID,
	) error

	// AddUserWithPasswordHash will add a new user to the database with the
	// provided password hash and salt. If the user already exists an error that
	// satisfies accesserrors.UserAlreadyExists will be returned. If the users creator
	// does not exist or has been previously removed an error that satisfies
	// accesserrors.UserCreatorUUIDNotFound will be returned.
	AddUserWithPasswordHash(
		ctx context.Context,
		uuid user.UUID,
		name user.Name,
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
		name user.Name,
		displayName string,
		creatorUUID user.UUID,
		permission permission.AccessSpec,
		activationKey []byte,
	) error

	// GetAllUsers will retrieve all users with authentication information
	// (last login, disabled) from the database. If no users exist an empty slice
	// will be returned.
	GetAllUsers(ctx context.Context, includeDisabled bool) ([]user.User, error)

	// GetUser will retrieve the user with authentication information (last login, disabled)
	// specified by UUID from the database. If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUser(context.Context, user.UUID) (user.User, error)

	// GetUserByName will retrieve the user with authentication information (last login, disabled)
	// specified by name from the database. If the user does not exist an error that satisfies
	// accesserrors.UserNotFound will be returned.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)

	// GetUserUUIDByName will retrieve the user UUID specified by name.
	// The following errors can be expected:
	// - [accesserrors.UserNotFound] when no user exists for the name.
	GetUserUUIDByName(ctx context.Context, name user.Name) (user.UUID, error)

	// GetUserByAuth will retrieve the user with checking authentication information
	// specified by name and password from the database. If the user does not exist
	// an error that satisfies accesserrors.UserNotFound will be returned.
	GetUserByAuth(context.Context, user.Name, auth.Password) (user.User, error)

	// RemoveUser marks the user as removed. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	// RemoveUser will also remove any credentials and activation codes for the
	// user. If no user exists for the given user name then an error that satisfies
	// accesserrors.UserNotFound will be returned.
	RemoveUser(context.Context, user.Name) error

	// SetActivationKey removes any active passwords for the user and sets the
	// activation key. If no user is found for the supplied user name an error
	// is returned that satisfies accesserrors.UserNotFound.
	SetActivationKey(context.Context, user.Name, []byte) error

	// GetActivationKey will retrieve the activation key for the user.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	GetActivationKey(context.Context, user.Name) ([]byte, error)

	// SetPasswordHash removes any active activation keys and sets the user
	// password hash and salt. If no user is found for the supplied user name an error
	// is returned that satisfies accesserrors.UserNotFound.
	SetPasswordHash(context.Context, user.Name, string, []byte) error

	// EnableUserAuthentication will enable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	EnableUserAuthentication(context.Context, user.Name) error

	// DisableUserAuthentication will disable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies accesserrors.UserNotFound.
	DisableUserAuthentication(context.Context, user.Name) error

	// UpdateLastModelLogin will update the last login time for the user.
	// The following error types are possible from this function:
	// - accesserrors.UserNameNotValid: When the username is not valid.
	// - accesserrors.UserNotFound: When the user cannot be found.
	// - modelerrors.NotFound: If no model by the given modelUUID exists.
	UpdateLastModelLogin(context.Context, user.Name, coremodel.UUID, time.Time) error

	// LastModelLogin will return the last login time of the specified user.
	// The following error types are possible from this function:
	// - accesserrors.UserNameNotValid: When the username is not valid.
	// - accesserrors.UserNotFound: When the user cannot be found.
	// - modelerrors.NotFound: If no model by the given modelUUID exists.
	// - accesserrors.UserNeverAccessedModel: If there is no record of the user
	// accessing the model.
	LastModelLogin(context.Context, user.Name, coremodel.UUID) (time.Time, error)
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
	DeletePermission(ctx context.Context, subject user.Name, target permission.ID) error

	// UpdatePermission updates the permission on the target for the given
	// subject (user). If a subject does not exist, it is created using the
	// subject and api user. Access can be granted or revoked.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error

	// ReadUserAccessForTarget returns the subject's (user) access for the
	// given user on the given target.
	ReadUserAccessForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.UserAccess, error)

	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	// If the access level of a user cannot be found then
	// accesserrors.AccessNotFound is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)

	// EnsureExternalUserIfAuthorized checks if an external user is missing from the database
	// and has permissions on an object. If they do then they will be added.
	// This ensures that juju has a record of external users that have inherited
	// their permissions from everyone@external.
	EnsureExternalUserIfAuthorized(ctx context.Context, subject user.Name, target permission.ID) error

	// ReadAllUserAccessForUser returns a slice of the user access the given
	// subject's (user) has for any access type.
	ReadAllUserAccessForUser(ctx context.Context, subject user.Name) ([]permission.UserAccess, error)

	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	ReadAllUserAccessForTarget(ctx context.Context, target permission.ID) ([]permission.UserAccess, error)

	// ReadAllAccessTypeForUser return a slice of user access for the subject
	// (user) specified and of the given object type.
	// E.G. All clouds the user has access to.
	ReadAllAccessForUserAndObjectType(ctx context.Context, subject user.Name, objectType permission.ObjectType) ([]permission.UserAccess, error)

	// AllModelAccessForCloudCredential for a given (cloud) credential key, return all
	// model name and model access levels.
	AllModelAccessForCloudCredential(ctx context.Context, key credential.Key) ([]access.CredentialOwnerModelAccess, error)
}

// Service provides the API for working with users.
type Service struct {
	*UserService
	*PermissionService
}

// NewService returns a new Service for interacting with the underlying access
// state.
func NewService(st State) *Service {
	return &Service{
		UserService:       NewUserService(st),
		PermissionService: NewPermissionService(st),
	}
}
