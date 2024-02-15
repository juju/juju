// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"

	"github.com/juju/errors"

	"github.com/juju/juju/core/user"
	domainuser "github.com/juju/juju/domain/user"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/auth"
)

// State describes retrieval and persistence methods for user identify and
// authentication.
type State interface {
	// AddUser will add a new user to the database. If the user already exists
	// an error that satisfies usererrors.AlreadyExists will be returned. If the
	// users creator is set and does not exist then an error that satisfies
	// usererrors.UserCreatorUUIDNotFound will be returned.
	AddUser(ctx context.Context, uuid user.UUID, name string, displayName string, creatorUUID user.UUID) error

	// AddUserWithPasswordHash will add a new user to the database with the
	// provided password hash and salt. If the user already exists an error that
	// satisfies usererrors.AlreadyExists will be returned. If the users creator
	// does not exist or has been previously removed an error that satisfies
	// usererrors.UserCreatorUUIDNotFound will be returned.
	AddUserWithPasswordHash(ctx context.Context, uuid user.UUID, name string, displayName string, creatorUUID user.UUID, passwordHash string, passwordSalt []byte) error

	// AddUserWithActivationKey will add a new user to the database with the
	// provided activation key. If the user already exists an error that
	// satisfies usererrors.AlreadyExists will be returned. if the users creator
	// does not exist or has been previously removed an error that satisfies
	// usererrors.UserCreatorUUIDNotFound will be returned.
	AddUserWithActivationKey(ctx context.Context, uuid user.UUID, name string, displayName string, creatorUUID user.UUID, activationKey []byte) error

	// GetAllUsers will retrieve all users with authentication information
	// (last login, disabled) from the database. If no users exist an empty slice
	// will be returned.
	GetAllUsers(context.Context) ([]user.User, error)

	// GetUser will retrieve the user with authentication information (last login, disabled)
	// specified by UUID from the database. If the user does not exist an error that satisfies
	// usererrors.NotFound will be returned.
	GetUser(context.Context, user.UUID) (user.User, error)

	// GetUserByName will retrieve the user with authentication information (last login, disabled)
	// specified by name from the database. If the user does not exist an error that satisfies
	// usererrors.NotFound will be returned.
	GetUserByName(ctx context.Context, name string) (user.User, error)

	// GetUserByAuth will retrieve the user with checking authentication information
	// specified by name and password from the database. If the user does not exist
	// an error that satisfies usererrors.NotFound will be returned.
	GetUserByAuth(context.Context, string, string) (user.User, error)

	// RemoveUser marks the user as removed. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	// RemoveUser will also remove any credentials and activation codes for the
	// user. If no user exists for the given UUID then an error that satisfies
	// usererrors.NotFound will be returned.
	RemoveUser(context.Context, user.UUID) error

	// SetActivationKey removes any active passwords for the user and sets the
	// activation key. If no user is found for the supplied UUID an error
	// is returned that satisfies usererrors.NotFound.
	SetActivationKey(context.Context, user.UUID, []byte) error

	// SetPasswordHash removes any active activation keys and sets the user
	// password hash and salt. If no user is found for the supplied UUID an error
	// is returned that satisfies usererrors.NotFound.
	SetPasswordHash(context.Context, user.UUID, string, []byte) error

	// EnableUserAuthentication will enable the user for authentication.
	// If no user is found for the supplied UUID an error is returned that
	// satisfies usererrors.NotFound.
	EnableUserAuthentication(context.Context, user.UUID) error

	// DisableUserAuthentication will disable the user for authentication.
	// If no user is found for the supplied UUID an error is returned that
	// satisfies usererrors.NotFound.
	DisableUserAuthentication(context.Context, user.UUID) error

	// UpdateLastLogin will update the last login time for the user.
	// If no user is found for the supplied UUID an error is returned that
	// satisfies usererrors.NotFound.
	UpdateLastLogin(context.Context, user.UUID) error
}

// Service provides the API for working with users.
type Service struct {
	st State
}

const (
	// activationKeyLength is the number of bytes contained with an activation
	// key.
	activationKeyLength = 32
)

// NewService returns a new Service for interacting with the underlying user
// state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// GetAllUsers will retrieve all users with authentication information
// (last login, disabled) from the database. If no users exist an empty slice
// will be returned.
func (s *Service) GetAllUsers(ctx context.Context) ([]user.User, error) {
	usrs, err := s.st.GetAllUsers(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting all users with auth info")
	}
	return usrs, nil
}

// GetUser will find and return the user with UUID. If there is no
// user for the UUID then an error that satisfies usererrors.NotFound will
// be returned.
func (s *Service) GetUser(
	ctx context.Context,
	uuid user.UUID,
) (user.User, error) {
	if err := uuid.Validate(); err != nil {
		return user.User{}, errors.Annotatef(usererrors.UUIDNotValid, "validating uuid %q", uuid)
	}

	usr, err := s.st.GetUser(ctx, uuid)
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user for uuid %q", uuid)
	}

	return usr, nil
}

// GetUserByName will find and return the user associated with name. If there is no
// user for the user name then an error that satisfies usererrors.NotFound will
// be returned. If supplied with an invalid user name then an error that satisfies
// usererrors.UsernameNotValid will be returned.
//
// GetUserByName will not return users that have been previously removed.
func (s *Service) GetUserByName(
	ctx context.Context,
	name string,
) (user.User, error) {
	if err := domainuser.ValidateUsername(name); err != nil {
		return user.User{}, errors.Annotatef(err, "validating username %q", name)
	}

	usr, err := s.st.GetUserByName(ctx, name)
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user %q", name)
	}

	return usr, nil
}

// GetUserByAuth will find and return the user with UUID. If there is no
// user for the name and password, then an error that satisfies
// usererrors.NotFound will be returned. If supplied with an invalid user name
// then an error that satisfies usererrors.UsernameNotValid will be returned.
//
// GetUserByAuth will not return users that have been previously removed.
func (s *Service) GetUserByAuth(
	ctx context.Context,
	name string,
	password string,
) (user.User, error) {
	if err := domainuser.ValidateUsername(name); err != nil {
		return user.User{}, errors.Annotatef(err, "validating username %q", name)
	}

	usr, err := s.st.GetUserByAuth(ctx, name, password)
	if err != nil {
		return user.User{}, errors.Annotatef(err, "getting user %q", name)
	}

	return usr, nil
}

// AddUser will add a new user to the database and return the UUID of the user.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
// - usererrors.UserCreatorUUIDNotFound: If a creator has been supplied for the user
// and the creator does not exist.
func (s *Service) AddUser(ctx context.Context, name string, displayName string, creatorUUID user.UUID) (user.UUID, error) {
	// Validate user name and creator name
	if err := domainuser.ValidateUsername(name); err != nil {
		return "", errors.Annotatef(err, "validating user name %q", name)
	}

	// Validate creator UUID
	if err := creatorUUID.Validate(); err != nil {
		return "", errors.Annotatef(err, "validating creator uuid %q", creatorUUID)
	}

	// Generate a UUID for the user.
	uuid, err := user.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "generating uuid for user %q", name)
	}

	if err = s.st.AddUser(ctx, uuid, name, displayName, creatorUUID); err != nil {
		return "", errors.Annotatef(err, "adding user %q", name)
	}
	return uuid, nil
}

// AddUserWithPassword will add a new user to the database with a password and return
// the UUID of the user. The password passed to this function will have
// it's Destroy() function called every time.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
// - usererrors.UserCreatorUUIDNotFound: If a creator has been supplied for the user
// and the creator does not exist.
// - internal/auth.ErrPasswordDestroyed: If the supplied password has already
// been destroyed.
// - internal/auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *Service) AddUserWithPassword(ctx context.Context, name string, displayName string, creatorUUID user.UUID, password auth.Password) (user.UUID, error) {
	defer password.Destroy()
	// Validate user name
	if err := domainuser.ValidateUsername(name); err != nil {
		return "", errors.Annotatef(err, "validating user name %q", name)
	}

	// Validate creator UUID
	if err := creatorUUID.Validate(); err != nil {
		return "", errors.Annotatef(err, "validating creator uuid %q", creatorUUID)
	}

	// Generate a salt for the password.
	salt, err := auth.NewSalt()
	if err != nil {
		return "", errors.Annotatef(err, "generating password salt for user %q", name)
	}

	// Hash the password.
	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return "", errors.Annotatef(err, "hashing password for user %q", name)
	}

	// Generate a UUID for the user.
	uuid, err := user.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "generating uuid for user %q", name)
	}

	if err = s.st.AddUserWithPasswordHash(ctx, uuid, name, displayName, creatorUUID, pwHash, salt); err != nil {
		return "", errors.Annotatef(err, "adding user %q with password hash", name)
	}
	return uuid, nil
}

// AddUserWithActivationKey will add a new user to the database with an activation key
// and return the UUID of the user.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
// - usererrors.UserCreatorUUIDNotFound: If a creator has been supplied for the user
// and the creator does not exist.
func (s *Service) AddUserWithActivationKey(ctx context.Context, name string, displayName string, creatorUUID user.UUID) ([]byte, user.UUID, error) {
	// Validate user name and creator name
	if err := domainuser.ValidateUsername(name); err != nil {
		return nil, "", errors.Annotatef(err, "validating user name %q", name)
	}

	// Validate creator UUID
	if err := creatorUUID.Validate(); err != nil {
		return nil, "", errors.Annotatef(err, "validating creator uuid %q", creatorUUID)
	}

	// Generate an activation key for the user.
	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, "", errors.Annotatef(err, "generating activation key for user %q", name)
	}

	// Generate a UUID for the user.
	uuid, err := user.NewUUID()
	if err != nil {
		return nil, "", errors.Annotatef(err, "generating uuid for user %q", name)
	}

	if err = s.st.AddUserWithActivationKey(ctx, uuid, name, displayName, creatorUUID, activationKey); err != nil {
		return nil, "", errors.Annotatef(err, "adding user %q with activation key", name)
	}
	return activationKey, uuid, nil
}

// RemoveUser marks the user as removed and removes any credentials or
// activation codes for the current users. Once a user is removed they are no
// longer usable in Juju and should never be un removed.
//
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) RemoveUser(ctx context.Context, uuid user.UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	if err := s.st.RemoveUser(ctx, uuid); err != nil {
		return errors.Annotatef(err, "removing user for uuid %q", uuid)
	}
	return nil
}

// SetPassword changes the users password to the new value and removes any
// active activation keys for the users. The password passed to this function
// will have it's Destroy() function called every time.
//
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given name exists.
// - internal/auth.ErrPasswordDestroyed: If the supplied password has already
// been destroyed.
// - internal/auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *Service) SetPassword(
	ctx context.Context,
	uuid user.UUID,
	password auth.Password,
) error {
	defer password.Destroy()
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return errors.Annotatef(err, "generating password salt for user with uuid %q", uuid)
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return errors.Annotatef(err, "hashing password for user with uuid %q", uuid)
	}

	if err = s.st.SetPasswordHash(ctx, uuid, pwHash, salt); err != nil {
		return errors.Annotatef(err, "setting password for user with uuid %q", uuid)
	}
	return nil
}

// ResetPassword will remove any active passwords for a user and generate a new
// activation key for the user to use to set a new password.

// The following error types are possible from this function:
// - usererrors.UUIDNitValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) ResetPassword(ctx context.Context, uuid user.UUID) ([]byte, error) {
	if err := uuid.Validate(); err != nil {
		return nil, errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, errors.Annotatef(err, "generating activation key for user with uuid %q", uuid)
	}

	if err = s.st.SetActivationKey(ctx, uuid, activationKey); err != nil {
		return nil, errors.Annotatef(err, "setting activation key for user with uuid %q", uuid)
	}
	return activationKey, nil
}

// EnableUserAuthentication will enable the user for authentication.
//
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) EnableUserAuthentication(ctx context.Context, uuid user.UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	if err := s.st.EnableUserAuthentication(ctx, uuid); err != nil {
		return errors.Annotatef(err, "enabling user with uuid %q", uuid)
	}
	return nil
}

// DisableUserAuthentication will disable the user for authentication.
//
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) DisableUserAuthentication(ctx context.Context, uuid user.UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	if err := s.st.DisableUserAuthentication(ctx, uuid); err != nil {
		return errors.Annotatef(err, "disabling user with uuid %q", uuid)
	}
	return nil
}

// UpdateLastLogin will update the last login time for the user.
//
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) UpdateLastLogin(ctx context.Context, uuid user.UUID) error {
	if err := uuid.Validate(); err != nil {
		return errors.Annotatef(usererrors.UUIDNotValid, "%q", uuid)
	}

	if err := s.st.UpdateLastLogin(ctx, uuid); err != nil {
		return errors.Annotatef(err, "updating last login for user with uuid %q", uuid)
	}
	return nil
}

// generateActivationKey is responsible for generating a new activation key that
// can be used for supplying to a user.
func generateActivationKey() ([]byte, error) {
	var activationKey [activationKeyLength]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, errors.Annotate(err, "generating activation key")
	}
	return activationKey[:], nil
}
