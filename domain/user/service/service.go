// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"

	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/user/errors"
)

// State describes retrieval and persistence methods for upgrade info.
type State interface {
	// AddUser will add a new user to the database. If the user already exists
	// an error that satisfies usererrors.AlreadyExists will be returned.
	AddUser(context.Context, string, string, string, string) error

	// GetUser will retrieve the user specified by name from the database where
	// the user is active and has not been deleted. If the user does not exist
	// or is deleted an error that satisfies usererrors.NotFound will be
	// returned.
	GetUser(context.Context, string) (user.User, error)
	// RemoveUser marks the user as deleted. This obviates the ability of a user
	// to function, but keeps the user retaining provenance, i.e. auditing.
	RemoveUser(context.Context, string) error
	// SetPassword removes any active activation keys and sets the user password
	SetPassword(context.Context, string, user.UserPassword) error
	// GenerateUserActivationKey makes a new activation key for the given user.
	// It replaces any existing key for the user.
	GenerateUserActivationKey(context.Context, string) error
	// RemoveUserActivationKey removes any activation key ass the given user
	RemoveUserActivationKey(context.Context, string) error
}

// Service provides the API for working with upgrade info
type Service struct {
	st State
}

const (
	// usernameValidationRegex is the regex used to validate that user names are
	// valid for consumption by Juju. Usernames must be 1 or more runes long,
	// can contain any unicode rune from the letter class and may contain zero
	// or more of .,+ or - runes as long as they are don't appear at the start
	// or end of the username.
	usernameValidationRegex = "^([\\pL\\pN]|[\\pL\\pN][\\pL\\pN.+-]{0,254}[\\pL\\pN])$"
)

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{st: st}
}

// GetUser will find and return the user associated with name. If there is no
// user for the user name then an error that satisfies usererrors.NotFound will
// be returned. If supplied with a invalid user name then a error that satisfies
// usererrors.
//
// GetUser will not return users that have been previously deleted or removed.
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
// Usernames must be 1 or more runes long, can contain any unicode rune from the
// letter class and may contain zero or more of .,+ or - runes as long as they
// are don't appear at the start or end of the username.
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

// AddUser will add a new user to the database. If the user already exists an
// error that satisfies usererrors.AlreadyExists will be returned.
func (s *Service) AddUser(ctx context.Context, name, displayName, password, creator string) error {
	err := ValidateUsername(name)
	if err != nil {
		return fmt.Errorf("username %q: %w", name, err)
	}

	err = s.st.AddUser(ctx, name, displayName, password, creator)
	if err != nil {
		return fmt.Errorf("adding user %q: %w", name, err)
	}
	return nil
}

// RemoveUser marks the user as deleted. This obviates the ability of a user
// to function, but keeps the userDoc retaining provenance, i.e. auditing.
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

// SetPassword removes any active activation keys and sets the user password
func (s *Service) SetPassword(ctx context.Context, name string,
	password user.UserPassword) error {
	if err := validateUserName(name); err != nil {
		return fmt.Errorf("username %q: %w", name, err)
	}
	lowercaseName := strings.ToLower(name)
	if err := s.st.RemoveUserActivationKey(ctx, lowercaseName); err != nil {
		return err
	}
	err := s.st.SetPassword(ctx, lowercaseName, password)
	if err != nil {
		return fmt.Errorf("unable to set password for user %q: %w", name, err)
	}
	return nil
}

// GenerateUserActivationKey makes a new activation key for the given user.
// It replaces any existing key for the user.
func (s *Service) GenerateUserActivationKey(ctx context.Context, name string) error {
	if err := validateUserName(name); err != nil {
		return fmt.Errorf("username %q: %w", name, err)
	}
	lowercaseName := strings.ToLower(name)
	err := s.st.GenerateUserActivationKey(ctx, lowercaseName)
	if err != nil {
		return fmt.Errorf("unable to generate activation key for user %q: %w", lowercaseName, err)
	}
	return nil
}
