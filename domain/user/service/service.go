// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"

	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/auth"
)

// State describes retrieval and persistence methods for user identify and
// authentication.
type State interface {
	// AddUser will add a new user to the database. If the user already exists
	// an error that satisfies usererrors.AlreadyExists will be returned.
	AddUser(context.Context, user.User) error

	// AddUserWithPassword will add a new user to the database with a password.
	//If the user already exists
	// an error that satisfies usererrors.AlreadyExists will be returned.
	AddUserWithPassword(context.Context, user.User, string, []byte) error

	// AddUserWithActivationKey will add a new user to the database with an activation key.
	// If the user already exists an error that satisfies usererrors.AlreadyExists
	// will be returned.
	AddUserWithActivationKey(context.Context, user.User, []byte) error

	// GetUser will retrieve the user specified by name from the database where
	// the user is active and has not been removed. If the user does not exist
	// or is deleted an error that satisfies usererrors.NotFound will be
	// returned.
	GetUser(context.Context, string) (user.User, error)

	// RemoveUser marks the user as removed. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	// RemoveUser will also remove any credentials and activation codes for the
	// user. If no user exists for the given name then a error that satisfies
	// usererrors.NotFound will be returned.
	RemoveUser(context.Context, string) error

	// SetActivationKey removes any active passwords for the user and sets the
	// activation key. If no user is found for the supplied name a error
	// is returned that satisfies usererrors.NotFound.
	SetActivationKey(context.Context, string, []byte) error

	// SetPasswordHash removes any active activation keys and sets the user
	// password hash and salt. If no user is found for the supplied name a error
	// is returned that satisfies usererrors.NotFound.
	SetPasswordHash(ctx context.Context, username string, passwordHash string, salt []byte) error
}

// Service provides the API for working with users.
type Service struct {
	st State
}

const (
	// activationKeyLength is the number of bytes contained with an activation
	// key.
	activationKeyLength = 32

	// usernameValidationRegex is the regex used to validate that user names are
	// valid for consumption by Juju. Usernames must be 1 or more runes long,
	// can contain any unicode rune from the letter/number class and may contain
	// zero or more of .,+ or - runes as long as they don't appear at the
	// start or end of the username. Usernames can be a maximum of 255
	// characters long.
	usernameValidationRegex = "^([\\pL\\pN]|[\\pL\\pN][\\pL\\pN.+-]{0,253}[\\pL\\pN])$"
)

// NewService returns a new Service for interacting with the underlying user
// state.
func NewService(st State) *Service {
	return &Service{st: st}
}

// GetUser will find and return the user associated with name. If there is no
// user for the user name then a error that satisfies usererrors.NotFound will
// be returned. If supplied with a invalid user name then a error that satisfies
// usererrors.UsernameNotValid will be returned.
//
// GetUser will not return users that have been previously removed.
func (s *Service) GetUser(
	ctx context.Context,
	name string,
) (user.User, error) {
	if err := ValidateUsername(name); err != nil {
		return user.User{}, fmt.Errorf("username %q: %w", name, err)
	}

	u, err := s.st.GetUser(ctx, name)
	if err != nil {
		return user.User{}, fmt.Errorf("getting user %q: %w", name, err)
	}

	return u, nil
}

// ValidateUsername takes a user name and validates that the user name is
// conformant to our regex rules defined in usernameValidationRegex. If a user
// name is not valid a error is returned that satisfies
// usererrors.UsernameNotValid.
//
// Usernames must be one or more runes long, can contain any unicode rune from
// the letter or number class and may contain zero or more of .,+ or - runes as
// long as they don't appear at the start or end of the username. Usernames can
// be a maximum length of 255 characters.
func ValidateUsername(name string) error {
	regex, err := regexp.Compile(usernameValidationRegex)
	if err != nil {
		return fmt.Errorf("compiling user name validation regex %q: %w",
			usernameValidationRegex, err,
		)
	}
	if !regex.MatchString(name) {
		return fmt.Errorf("%w %q", usererrors.UsernameNotValid, name)
	}
	return nil
}

// AddUser will add a new user to the database.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
func (s *Service) AddUser(ctx context.Context, user user.User) error {
	if err := ValidateUsername(user.Name); err != nil {
		return fmt.Errorf("username %q: %w", user.Name, err)
	}

	if err := s.st.AddUser(ctx, user); err != nil {
		return fmt.Errorf("adding user %q: %w", user.Name, err)
	}
	return nil
}

// AddUserWithPassword will add a new user to the database with a password.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
func (s *Service) AddUserWithPassword(ctx context.Context, user user.User, password auth.Password) error {
	if err := ValidateUsername(user.Name); err != nil {
		return fmt.Errorf("username %q with password: %w", user.Name, err)
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return fmt.Errorf("setting password for user %q, generating password salt: %w", user.Name, err)
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return fmt.Errorf("setting password for user %q, hashing password: %w", user.Name, err)
	}

	if err = s.st.AddUserWithPassword(ctx, user, pwHash, salt); err != nil {
		return fmt.Errorf("adding user %q with password: %w", user.Name, err)
	}
	// Destroy the password before return.
	password.Destroy()
	return nil
}

// AddUserWithActivationKey will add a new user to the database with an activation key.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.AlreadyExists: If a user with the supplied name already exists.
func (s *Service) AddUserWithActivationKey(ctx context.Context, user user.User) ([]byte, error) {
	err := ValidateUsername(user.Name)
	if err != nil {
		return nil, fmt.Errorf("username %q with activation key: %w", user.Name, err)
	}

	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, fmt.Errorf("generating activation key for user %q: %w", user.Name, err)
	}

	err = s.st.AddUserWithActivationKey(ctx, user, activationKey)
	if err != nil {
		return nil, fmt.Errorf("adding user %q with activation key: %w", user.Name, err)
	}
	return activationKey, nil
}

// RemoveUser marks the user as removed and removes any credentials or
// activation codes for the current users. Once a user is removed they are no
// longer usable in Juju and should never be un removed.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given name exists.
func (s *Service) RemoveUser(ctx context.Context, name string) error {
	if err := ValidateUsername(name); err != nil {
		return fmt.Errorf("username %q: %w", name, err)
	}
	err := s.st.RemoveUser(ctx, name)
	if err != nil {
		return fmt.Errorf("removing user %q: %w", name, err)
	}
	return nil
}

// SetPassword changes the users password to the new value and removes any
// active activation keys for the users. The password passed to this function
// will have it's Destroy() function called every time.
//
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given name exists.
// - internal/auth.ErrPasswordDestroyed: If the supplied password has already
// been destroyed.
// - internal/auth.ErrPasswordNotValid: If the password supplied is not valid.
func (s *Service) SetPassword(
	ctx context.Context,
	name string,
	password auth.Password,
) error {
	defer password.Destroy()
	if err := ValidateUsername(name); err != nil {
		return fmt.Errorf("username %q: %w", name, err)
	}

	salt, err := auth.NewSalt()
	if err != nil {
		return fmt.Errorf("setting password for user %q, generating password salt: %w", name, err)
	}

	pwHash, err := auth.HashPassword(password, salt)
	if err != nil {
		return fmt.Errorf("setting password for user %q, hashing password: %w", name, err)
	}

	err = s.st.SetPasswordHash(ctx, name, pwHash, salt)
	if err != nil {
		return fmt.Errorf("setting password for user %q: %w", name, err)
	}
	return nil
}

// ResetPassword will remove any active passwords for a user and generate a new
// activation key for the user to use to set a new password.
// The following error types are possible from this function:
// - usererrors.UsernameNotValid: When the username supplied is not valid.
// - usererrors.NotFound: If no user by the given name exists.
func (s *Service) ResetPassword(ctx context.Context, name string) ([]byte, error) {
	if err := ValidateUsername(name); err != nil {
		return nil, fmt.Errorf("username %q: %w", name, err)
	}

	activationKey, err := generateActivationKey()
	if err != nil {
		return nil, fmt.Errorf("generating activation key for user %q: %w", name, err)
	}

	if err = s.st.SetActivationKey(ctx, name, activationKey); err != nil {
		return nil, fmt.Errorf("setting activation key for user %q: %w", name, err)
	}
	return activationKey, nil
}

// generateActivationKey is responsible for generating a new activation key that
// can be used for supplying to a user.
func generateActivationKey() ([]byte, error) {
	var activationKey [activationKeyLength]byte
	if _, err := rand.Read(activationKey[:]); err != nil {
		return nil, err
	}
	return activationKey[:], nil
}
