// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/juju/juju/core/permission"
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
	// usererrors.CreatorUUIDNotFound will be returned.
	AddUser(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.UserPermissionAccess,
	) error

	// AddUserWithPasswordHash will add a new user to the database with the
	// provided password hash and salt. If the user already exists an error that
	// satisfies usererrors.AlreadyExists will be returned. If the users creator
	// does not exist or has been previously removed an error that satisfies
	// usererrors.CreatorUUIDNotFound will be returned.
	AddUserWithPasswordHash(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.UserPermissionAccess,
		passwordHash string,
		passwordSalt []byte,
	) error

	// AddUserWithActivationKey will add a new user to the database with the
	// provided activation key. If the user already exists an error that
	// satisfies usererrors.AlreadyExists will be returned. if the users creator
	// does not exist or has been previously removed an error that satisfies
	// usererrors.CreatorUUIDNotFound will be returned.
	AddUserWithActivationKey(
		ctx context.Context,
		uuid user.UUID,
		name string,
		displayName string,
		creatorUUID user.UUID,
		permission permission.UserPermissionAccess,
		activationKey []byte,
	) error

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
	GetUserByAuth(context.Context, string, auth.Password) (user.User, error)

	// RemoveUser marks the user as removed. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	// RemoveUser will also remove any credentials and activation codes for the
	// user. If no user exists for the given user name then an error that satisfies
	// usererrors.NotFound will be returned.
	RemoveUser(context.Context, string) error

	// SetActivationKey removes any active passwords for the user and sets the
	// activation key. If no user is found for the supplied user name an error
	// is returned that satisfies usererrors.NotFound.
	SetActivationKey(context.Context, string, []byte) error

	// GetActivationKey will retrieve the activation key for the user.
	// If no user is found for the supplied user name an error is returned that
	// satisfies usererrors.NotFound.
	GetActivationKey(context.Context, string) ([]byte, error)

	// SetPasswordHash removes any active activation keys and sets the user
	// password hash and salt. If no user is found for the supplied user name an error
	// is returned that satisfies usererrors.NotFound.
	SetPasswordHash(context.Context, string, string, []byte) error

	// EnableUserAuthentication will enable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies usererrors.NotFound.
	EnableUserAuthentication(context.Context, string) error

	// DisableUserAuthentication will disable the user for authentication.
	// If no user is found for the supplied user name an error is returned that
	// satisfies usererrors.NotFound.
	DisableUserAuthentication(context.Context, string) error

	// UpdateLastLogin will update the last login time for the user.
	// If no user is found for the supplied user name an error is returned that
	// satisfies usererrors.NotFound.
	UpdateLastLogin(context.Context, string) error
}

// Service provides the API for working with users.
type Service struct {
	st State
}

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
// usererrors.UserNameNotValid will be returned.
//
// GetUserByName will not return users that have been previously removed.
func (s *Service) GetUserByName(
	ctx context.Context,
	name string,
) (user.User, error) {
	if err := domainuser.ValidateUserName(name); err != nil {
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
// then an error that satisfies usererrors.UserNameNotValid will be returned.
// It will not return users that have been previously removed.
func (s *Service) GetUserByAuth(
	ctx context.Context,
	name string,
	password auth.Password,
) (user.User, error) {
	if err := domainuser.ValidateUserName(name); err != nil {
		return user.User{}, errors.Annotatef(err, "validating username %q", name)
	}
	if err := password.Validate(); err != nil {
		return user.User{}, errors.Trace(err)
	}

	usr, err := s.st.GetUserByAuth(ctx, name, password)
	if err != nil {
		// We only need to ensure destruction on an error.
		// The happy path hashes the password in state,
		// and in so doing destroys it.
		password.Destroy()
		return user.User{}, errors.Annotatef(err, "getting user %q", name)
	}

	return usr, nil
}

// AddUser will add a new user to the database and return the UUID of the
// user if successful. If no password is set in the incoming argument,
// the user will be added with an activation key.
// The following error types are possible from this function:
//   - usererrors.UserNameNotValid: When the username supplied is not valid.
//   - usererrors.AlreadyExists: If a user with the supplied name already exists.
//   - usererrors.CreatorUUIDNotFound: If a creator has been supplied for the user
//     and the creator does not exist.
//   - auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *Service) AddUser(ctx context.Context, arg AddUserArg) (user.UUID, []byte, error) {
	if err := domainuser.ValidateUserName(arg.Name); err != nil {
		return "", nil, errors.Annotatef(err, "validating user name %q", arg.Name)
	}

	if err := arg.CreatorUUID.Validate(); err != nil {
		return "", nil, errors.Annotatef(err, "validating creator UUID %q", arg.CreatorUUID)
	}

	if err := arg.Permission.Validate(); err != nil {
		return "", nil, fmt.Errorf("validating permission %q: %w: %w", arg.Permission, err, usererrors.PermissionNotValid)
	}

	if arg.UUID.String() == "" {
		var err error
		if arg.UUID, err = user.NewUUID(); err != nil {
			return "", nil, errors.Annotatef(err, "generating UUID for user %q", arg.Name)
		}
	} else if err := arg.UUID.Validate(); err != nil {
		return "", nil, errors.Annotatef(err, "validating user UUID %q", arg.UUID)
	}

	var key []byte
	var err error
	if arg.Password != nil {
		err = s.addUserWithPassword(ctx, arg)
	} else {
		key, err = s.addUserWithActivationKey(ctx, arg)
	}
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	return arg.UUID, key, nil
}

func (s *Service) addUserWithPassword(ctx context.Context, arg AddUserArg) error {
	if err := arg.Password.Validate(); err != nil {
		return errors.Trace(err)
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return errors.Trace(err)
	}

	hash, err := auth.HashPassword(*arg.Password, salt)
	if err != nil {
		return errors.Trace(err)
	}

	err = s.st.AddUserWithPasswordHash(ctx, arg.UUID, arg.Name, arg.DisplayName, arg.CreatorUUID, arg.Permission, hash, salt)
	return errors.Trace(err)
}

func (s *Service) addUserWithActivationKey(ctx context.Context, arg AddUserArg) ([]byte, error) {
	key, err := generateActivationKey()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.st.AddUserWithActivationKey(ctx, arg.UUID, arg.Name, arg.DisplayName, arg.CreatorUUID, arg.Permission, key); err != nil {
		return nil, errors.Trace(err)
	}
	return key, nil
}

// RemoveUser marks the user as removed and removes any credentials or
// activation codes for the current users. Once a user is removed they are no
// longer usable in Juju and should never be un removed.
// The following error types are possible from this function:
// - usererrors.UserNameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) RemoveUser(ctx context.Context, name string) error {
	if err := domainuser.ValidateUserName(name); err != nil {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	if err := s.st.RemoveUser(ctx, name); err != nil {
		return errors.Annotatef(err, "removing user for %q", name)
	}
	return nil
}

// SetPassword changes the users password to the new value and removes any
// active activation keys for the users.
// The following error types are possible from this function:
//   - usererrors.UserNameNotValid: When the username supplied is not valid.
//   - usererrors.NotFound: If no user by the given name exists.
//   - internal/auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *Service) SetPassword(ctx context.Context, name string, pass auth.Password) error {
	if err := domainuser.ValidateUserName(name); err != nil {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	if err := pass.Validate(); err != nil {
		return errors.Trace(err)
	}

	err := s.setPassword(ctx, name, pass)
	return errors.Trace(err)
}

// ResetPassword will remove any active passwords for a user and generate a new
// activation key for the user to use to set a new password.
// The following error types are possible from this function:
// - usererrors.UserNameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) ResetPassword(ctx context.Context, name string) ([]byte, error) {
	if err := domainuser.ValidateUserName(name); err != nil {
		return nil, errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.st.SetActivationKey(ctx, name, activationKey); err != nil {
		return nil, errors.Annotatef(err, "setting activation key for user %q", name)
	}
	return activationKey, nil
}

// EnableUserAuthentication will enable the user for authentication.
// The following error types are possible from this function:
// - usererrors.UserNameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) EnableUserAuthentication(ctx context.Context, name string) error {
	if err := domainuser.ValidateUserName(name); err != nil {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	if err := s.st.EnableUserAuthentication(ctx, name); err != nil {
		return errors.Annotatef(err, "enabling user with uuid %q", name)
	}
	return nil
}

// DisableUserAuthentication will disable the user for authentication.
// The following error types are possible from this function:
// - usererrors.UserNameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) DisableUserAuthentication(ctx context.Context, name string) error {
	if err := domainuser.ValidateUserName(name); err != nil {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	if err := s.st.DisableUserAuthentication(ctx, name); err != nil {
		return errors.Annotatef(err, "disabling user %q", name)
	}
	return nil
}

// UpdateLastLogin will update the last login time for the user.
// The following error types are possible from this function:
// - usererrors.UUIDNotValid: When the UUID supplied is not valid.
// - usererrors.NotFound: If no user by the given UUID exists.
func (s *Service) UpdateLastLogin(ctx context.Context, name string) error {
	if err := domainuser.ValidateUserName(name); err != nil {
		return errors.Annotatef(usererrors.UserNameNotValid, "%q", name)
	}

	if err := s.st.UpdateLastLogin(ctx, name); err != nil {
		return errors.Annotatef(err, "updating last login for user %q", name)
	}
	return nil
}

// activationKeyLength is the number of bytes in an activation key.
const activationKeyLength = 32

// generateActivationKey is responsible for generating a new activation key that
// can be used for supplying to a user.
func generateActivationKey() ([]byte, error) {
	var activationKey [activationKeyLength]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, errors.Annotate(err, "generating activation key")
	}
	return activationKey[:], nil
}

// activationBoxNonceLength is the number of bytes in the nonce for the
// activation box.
const activationBoxNonceLength = 24

// Sealer is an interface that can be used to seal a byte slice.
// This will use the nonce and box for a given user to seal the payload.
type Sealer interface {
	// Seal will seal the payload using the nonce and box for the user.
	Seal(nonce, payload []byte) ([]byte, error)
}

// SetPasswordWithActivationKey will use the activation key from the user. To
// then apply the payload password. If the user does not exist an error that
// satisfies usererrors.NotFound will be return. If the nonce is not the
// correct length an error that satisfies errors.NotValid will be returned.
//
// This will use the NaCl secretbox to open the box and then unmarshal the
// payload to set the new password for the user. If the payload cannot be
// unmarshalled an error will be returned.
// To prevent the leaking of the key and nonce (which can unbox the secret),
// a Sealer will be returned that can be used to seal the response payload.
func (s *Service) SetPasswordWithActivationKey(ctx context.Context, name string, nonce, box []byte) (Sealer, error) {
	if len(nonce) != activationBoxNonceLength {
		return nil, errors.NotValidf("nonce")
	}

	// Get the activation key for the user.
	key, err := s.st.GetActivationKey(ctx, name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Copy the nonce and the key to arrays which can be used for the secretbox.
	var sbKey [activationKeyLength]byte
	var sbNonce [activationBoxNonceLength]byte
	copy(sbKey[:], key)
	copy(sbNonce[:], nonce)

	// The box is the payload that has been sealed with the nonce and key, so
	// let's open it.
	boxPayloadBytes, ok := secretbox.Open(nil, box, &sbNonce, &sbKey)
	if !ok {
		return nil, usererrors.ActivationKeyNotValid
	}

	// We expect the payload to be a JSON object with a password field.
	var payload struct {
		// Password is the new password to set for the user.
		Password string `json:"password"`
	}
	if err := json.Unmarshal(boxPayloadBytes, &payload); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal payload")
	}

	if err := s.setPassword(ctx, name, auth.NewPassword(payload.Password)); err != nil {
		return nil, errors.Annotate(err, "setting new password")
	}

	return boxSealer{
		key: sbKey,
	}, nil
}

func (s *Service) setPassword(ctx context.Context, name string, pass auth.Password) error {
	salt, err := auth.NewSalt()
	if err != nil {
		return errors.Annotatef(err, "generating password salt for user %q", name)
	}

	pwHash, err := auth.HashPassword(pass, salt)
	if err != nil {
		return errors.Annotatef(err, "hashing password for user %q", name)
	}

	if err = s.st.SetPasswordHash(ctx, name, pwHash, salt); err != nil {
		return errors.Annotatef(err, "setting password for user %q", name)
	}

	return nil
}

// boxSealer is a Sealer that uses the NaCl secretbox to seal a payload.
type boxSealer struct {
	key [activationKeyLength]byte
}

func (s boxSealer) Seal(nonce, payload []byte) ([]byte, error) {
	if len(nonce) != activationBoxNonceLength {
		return nil, errors.NotValidf("nonce")
	}

	var sbNonce [activationBoxNonceLength]byte
	copy(sbNonce[:], nonce)
	return secretbox.Seal(nil, payload, &sbNonce, &s.key), nil
}
