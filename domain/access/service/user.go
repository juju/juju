// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"time"

	"golang.org/x/crypto/nacl/secretbox"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/errors"
)

// UserService provides the API for working with users.
type UserService struct {
	st UserState
}

// NewUserService returns a new UserService for interacting with the underlying user
// state.
func NewUserService(st UserState) *UserService {
	return &UserService{
		st: st,
	}
}

// GetAllUsers will retrieve all users with authentication information
// (last login, disabled) from the database. If no users exist an empty slice
// will be returned.
func (s *UserService) GetAllUsers(ctx context.Context, includeDisabled bool) (_ []user.User, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	usrs, err := s.st.GetAllUsers(ctx, includeDisabled)
	if err != nil {
		return nil, errors.Errorf("getting all users with auth info: %w", err)
	}
	return usrs, nil
}

// GetUser will find and return the user with UUID. If there is no
// user for the UUID then an error that satisfies accesserrors.NotFound will
// be returned.
func (s *UserService) GetUser(
	ctx context.Context,
	uuid user.UUID,
) (_ user.User, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return user.User{}, errors.Errorf("validating uuid %q: %w", uuid, accesserrors.UserUUIDNotValid)
	}

	usr, err := s.st.GetUser(ctx, uuid)
	if err != nil {
		return user.User{}, errors.Errorf("getting user for uuid %q: %w", uuid, err)
	}

	return usr, nil
}

// GetUserByName will find and return the user associated with name. If there is no
// user for the user name then an error that satisfies accesserrors.NotFound will
// be returned. If supplied with an invalid user name then an error that satisfies
// accesserrors.UserNameNotValid will be returned.
//
// GetUserByName will not return users that have been previously removed.
func (s *UserService) GetUserByName(
	ctx context.Context,
	name user.Name,
) (_ user.User, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return user.User{}, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	usr, err := s.st.GetUserByName(ctx, name)
	if err != nil {
		return user.User{}, errors.Errorf("getting user %q: %w", name, err)
	}

	return usr, nil
}

// GetUserUUIDByName will find and return the UUID associated with user name.
// The following errors can be expected:
// - [accesserrors.UserNotFound] when no user exists for the supplied user name.
// - [accesserrors.UserNameNotValid] when the user name is not valid.
func (s *UserService) GetUserUUIDByName(
	ctx context.Context,
	name user.Name,
) (_ user.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return "", errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	uuid, err := s.st.GetUserUUIDByName(ctx, name)
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetUserByAuth will find and return the user with UUID. If there is no
// user for the name and password, then an error that satisfies
// accesserrors.NotFound will be returned. If supplied with an invalid user name
// then an error that satisfies accesserrors.UserNameNotValid will be returned.
// It will not return users that have been previously removed.
func (s *UserService) GetUserByAuth(
	ctx context.Context,
	name user.Name,
	password auth.Password,
) (_ user.User, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return user.User{}, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := password.Validate(); err != nil {
		return user.User{}, errors.Capture(err)
	}

	usr, err := s.st.GetUserByAuth(ctx, name, password)
	if err != nil {
		// We only need to ensure destruction on an error.
		// The happy path hashes the password in state,
		// and in so doing destroys it.
		password.Destroy()
		return user.User{}, errors.Errorf("getting user %q: %w", name, err)
	}

	return usr, nil
}

// AddUser will add a new user to the database and return the UUID of the
// user if successful. If no password is set in the incoming argument,
// the user will be added with an activation key.
// The following error types are possible from this function:
//   - accesserrors.UserNameNotValid: When the username supplied is not valid.
//   - accesserrors.UserAlreadyExists: If a user with the supplied name already exists.
//   - accesserrors.CreatorUUIDNotFound: If a creator has been supplied for the user
//     and the creator does not exist.
//   - auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *UserService) AddUser(ctx context.Context, arg AddUserArg) (_ user.UUID, _ []byte, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if arg.Name.IsZero() {
		return "", nil, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}
	if !arg.Name.IsLocal() {
		return "", nil, errors.Errorf("cannot add external user %q: %w", arg.Name, accesserrors.UserNameNotValid)
	}
	if err := arg.CreatorUUID.Validate(); err != nil {
		return "", nil, errors.Errorf("validating creator UUID %q: %w", arg.CreatorUUID, err)
	}

	if err := arg.Permission.Validate(); err != nil {
		return "", nil, errors.Errorf("validating permission %q: %w: %w", arg.Permission, err, accesserrors.PermissionNotValid)
	}

	if arg.UUID.String() == "" {
		var err error
		if arg.UUID, err = user.NewUUID(); err != nil {
			return "", nil, errors.Errorf("generating UUID for user %q: %w", arg.Name, err)
		}
	} else if err := arg.UUID.Validate(); err != nil {
		return "", nil, errors.Errorf("validating user UUID %q: %w", arg.UUID, err)
	}

	var key []byte
	if arg.Password != nil {
		err = s.addUserWithPassword(ctx, arg)
	} else {
		key, err = s.addUserWithActivationKey(ctx, arg)
	}
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	return arg.UUID, key, nil
}

func (s *UserService) addUserWithPassword(ctx context.Context, arg AddUserArg) error {
	if err := arg.Password.Validate(); err != nil {
		return errors.Capture(err)
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return errors.Capture(err)
	}

	hash, err := auth.HashPassword(*arg.Password, salt)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.st.AddUserWithPasswordHash(ctx, arg.UUID, arg.Name, arg.DisplayName, arg.CreatorUUID, arg.Permission, hash, salt)
	return errors.Capture(err)
}

func (s *UserService) addUserWithActivationKey(ctx context.Context, arg AddUserArg) ([]byte, error) {
	key, err := generateActivationKey()
	if err != nil {
		return nil, errors.Capture(err)
	}

	if err := s.st.AddUserWithActivationKey(ctx, arg.UUID, arg.Name, arg.DisplayName, arg.CreatorUUID, arg.Permission, key); err != nil {
		return nil, errors.Capture(err)
	}
	return key, nil
}

// AddExternalUser adds a new external user to the database and does not set a
// password or activation key.
// The following error types are possible from this function:
//   - accesserrors.UserNameNotValid: When the username supplied is not valid.
//   - accesserrors.UserAlreadyExists: If a user with the supplied name already exists.
//   - accesserrors.CreatorUUIDNotFound: If the creator supplied for the user
//     does not exist.
func (s *UserService) AddExternalUser(ctx context.Context, name user.Name, displayName string, creatorUUID user.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsLocal() {
		return errors.Errorf("cannot use add external user method to add local user: %w", accesserrors.UserNameNotValid)
	}

	uuid, err := user.NewUUID()
	if err != nil {
		return errors.Errorf("generating user UUID: %w", err)
	}
	return s.st.AddUser(ctx, uuid, name, displayName, true, creatorUUID)
}

// RemoveUser marks the user as removed and removes any credentials or
// activation codes for the current users. Once a user is removed they are no
// longer usable in Juju and should never be un removed.
// The following error types are possible from this function:
// - accesserrors.UserNameNotValid: When the username supplied is not valid.
// - accesserrors.NotFound: If no user by the given UUID exists.
func (s *UserService) RemoveUser(ctx context.Context, name user.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}
	if err := s.st.RemoveUser(ctx, name); err != nil {
		return errors.Errorf("removing user for %q: %w", name, err)
	}
	return nil
}

// SetPassword changes the users password to the new value and removes any
// active activation keys for the users.
// The following error types are possible from this function:
//   - accesserrors.UserNameNotValid: When the username supplied is not valid.
//   - accesserrors.NotFound: If no user by the given name exists.
//   - internal/auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *UserService) SetPassword(ctx context.Context, name user.Name, pass auth.Password) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := pass.Validate(); err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(s.setPassword(ctx, name, pass))
}

// ResetPassword will remove any active passwords for a user and generate a new
// activation key for the user to use to set a new password.
// The following error types are possible from this function:
// - accesserrors.UserNameNotValid: When the username supplied is not valid.
// - accesserrors.NotFound: If no user by the given UUID exists.
func (s *UserService) ResetPassword(ctx context.Context, name user.Name) (_ []byte, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if name.IsZero() {
		return nil, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, errors.Capture(err)
	}

	if err = s.st.SetActivationKey(ctx, name, activationKey); err != nil {
		return nil, errors.Errorf("setting activation key for user %q: %w", name, err)
	}
	return activationKey, nil
}

// EnableUserAuthentication will enable the user for authentication.
// The following error types are possible from this function:
// - accesserrors.UserNameNotValid: When the username supplied is not valid.
// - accesserrors.NotFound: If no user by the given UUID exists.
func (s *UserService) EnableUserAuthentication(ctx context.Context, name user.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := s.st.EnableUserAuthentication(ctx, name); err != nil {
		return errors.Errorf("enabling user with uuid %q: %w", name, err)
	}
	return nil
}

// DisableUserAuthentication will disable the user for authentication.
// The following error types are possible from this function:
// - accesserrors.UserNameNotValid: When the username supplied is not valid.
// - accesserrors.NotFound: If no user by the given UUID exists.
func (s *UserService) DisableUserAuthentication(ctx context.Context, name user.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := s.st.DisableUserAuthentication(ctx, name); err != nil {
		return errors.Errorf("disabling user %q: %w", name, err)
	}
	return nil
}

// UpdateLastModelLogin will update the last login time for the user.
// The following error types are possible from this function:
// - [accesserrors.UserNameNotValid] when the username supplied is not valid.
// - [accesserrors.UserNotFound] when the user cannot be found.
// - [modelerrors.NotFound] if no model by the given modelUUID exists.
func (s *UserService) UpdateLastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := s.st.UpdateLastModelLogin(ctx, name, modelUUID, time.Now()); err != nil {
		return errors.Errorf("updating last login for user %q: %w", name, err)
	}
	return nil
}

// SetLastModelLogin will set the last login time for the user to the given
// value. The following error types are possible from this function:
// [accesserrors.UserNameNotValid] when the username supplied is not valid.
// [accesserrors.UserNotFound] when the user cannot be found.
// [modelerrors.NotFound] if no model by the given modelUUID exists.
func (s *UserService) SetLastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID, lastLogin time.Time) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := s.st.UpdateLastModelLogin(ctx, name, modelUUID, lastLogin); err != nil {
		return errors.Errorf("setting last login for user %q: %w", name, err)
	}
	return nil
}

// LastModelLogin will return the last login time of the specified user.
// The following error types are possible from this function:
// - [accesserrors.UserNameNotValid] when the username is not valid.
// - [accesserrors.UserNotFound] when the user cannot be found.
// - [modelerrors.NotFound] if no model by the given modelUUID exists.
// - [accesserrors.UserNeverAccessedModel] if there is no record of the user
// accessing the model.
func (s *UserService) LastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID) (_ time.Time, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return time.Time{}, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if err := modelUUID.Validate(); err != nil {
		return time.Time{}, errors.Errorf("getting last model connection for %q: bad uuid: %w", name, err)
	}

	lastConnection, err := s.st.LastModelLogin(ctx, name, modelUUID)
	if err != nil {
		return time.Time{}, errors.Capture(err)
	}
	return lastConnection, nil
}

// activationKeyLength is the number of bytes in an activation key.
const activationKeyLength = 32

// generateActivationKey is responsible for generating a new activation key that
// can be used for supplying to a user.
func generateActivationKey() ([]byte, error) {
	var activationKey [activationKeyLength]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, errors.Errorf("generating activation key: %w", err)
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
// satisfies accesserrors.NotFound will be returned. If the nonce is not the
// correct length an error that satisfies errors.NotValid will be returned.
//
// This will use the NaCl secretbox to open the box and then unmarshal the
// payload to set the new password for the user. If the payload cannot be
// unmarshalled an error will be returned.
// To prevent the leaking of the key and nonce (which can unbox the secret),
// a Sealer will be returned that can be used to seal the response payload.
func (s *UserService) SetPasswordWithActivationKey(ctx context.Context, name user.Name, nonce, box []byte) (_ Sealer, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if name.IsZero() {
		return nil, errors.Errorf("empty username: %w", accesserrors.UserNameNotValid)
	}

	if len(nonce) != activationBoxNonceLength {
		return nil, errors.Errorf("nonce %w", coreerrors.NotValid)
	}

	// Get the activation key for the user.
	key, err := s.st.GetActivationKey(ctx, name)
	if err != nil {
		return nil, errors.Capture(err)
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
		return nil, accesserrors.ActivationKeyNotValid
	}

	// We expect the payload to be a JSON object with a password field.
	var payload struct {
		// Password is the new password to set for the user.
		Password string `json:"password"`
	}
	if err := json.Unmarshal(boxPayloadBytes, &payload); err != nil {
		return nil, errors.Errorf("cannot unmarshal payload: %w", err)
	}

	if err := s.setPassword(ctx, name, auth.NewPassword(payload.Password)); err != nil {
		return nil, errors.Errorf("setting new password: %w", err)
	}

	return boxSealer{
		key: sbKey,
	}, nil
}

func (s *UserService) setPassword(ctx context.Context, name user.Name, pass auth.Password) error {
	salt, err := auth.NewSalt()
	if err != nil {
		return errors.Errorf("generating password salt for user %q: %w", name, err)
	}

	pwHash, err := auth.HashPassword(pass, salt)
	if err != nil {
		return errors.Errorf("hashing password for user %q: %w", name, err)
	}

	if err = s.st.SetPasswordHash(ctx, name, pwHash, salt); err != nil {
		return errors.Errorf("setting password for user %q: %w", name, err)
	}

	return nil
}

// boxSealer is a Sealer that uses the NaCl secretbox to seal a payload.
type boxSealer struct {
	key [activationKeyLength]byte
}

func (s boxSealer) Seal(nonce, payload []byte) ([]byte, error) {
	if len(nonce) != activationBoxNonceLength {
		return nil, errors.Errorf("nonce %w", coreerrors.NotValid)
	}

	var sbNonce [activationBoxNonceLength]byte
	copy(sbNonce[:], nonce)
	return secretbox.Seal(nil, payload, &sbNonce, &s.key), nil
}
