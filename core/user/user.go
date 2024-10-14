// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"regexp"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

const (
	LocalUserDomain = "local"
)

// User represents a user in the system.
type User struct {
	// UUID is the unique identifier for the user.
	UUID UUID

	// Name is the username of the user.
	Name Name

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string

	// CreatorUUID is the associated user that created this user.
	CreatorUUID UUID

	// CreatorName is the name of the user that created this user.
	CreatorName Name

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// LastLogin is the last time the user logged in.
	LastLogin time.Time

	// Disabled is true if the user is disabled.
	Disabled bool
}

// UUID is a unique identifier for a user.
type UUID string

// NewUUID returns a new UUID.
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	return UUID(uuid.String()), nil
}

// ParseUUID parses a UUID from a string. If the string is not a valid UUID
// an error satisfying [errors.NotValid] will be returned.
func ParseUUID(u string) (UUID, error) {
	if !uuid.IsValidUUIDString(u) {
		return "", errors.NotValidf("uuid %q", u)
	}
	return UUID(u), nil
}

// Validate returns an error if the UUID is invalid. The error returned
// satisfies [errors.NotValid].
func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("empty uuid%w", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("invalid uuid: %q%w", u, errors.Hide(errors.NotValid))
	}
	return nil
}

// String returns the UUID as a string.
func (u UUID) String() string {
	return string(u)
}

// userNameTag is the name of the user.
type userNameTag interface {
	// Name returns the name of the user.
	Name() string
	// Domain returns the domain of the user.
	Domain() string
}

var (
	validUserNameSnippet = "[a-zA-Z0-9][a-zA-Z0-9.+-]*[a-zA-Z0-9]"
	validName            = regexp.MustCompile(fmt.Sprintf("^(?P<name>%s)(?:@(?P<domain>%s))?$", validUserNameSnippet, validUserNameSnippet))
)

// NewName validates the name and returns a new Name object. If the name is not
// valid an error satisfying [errors.NotValid] will be returned.
func NewName(name string) (Name, error) {
	parts := validName.FindStringSubmatch(name)
	if len(parts) != 3 {
		return Name{}, errors.NotValidf("user name %q", name)
	}
	domain := parts[2]
	if domain == LocalUserDomain {
		domain = ""
	}
	return Name{
		name:   parts[1],
		domain: domain,
	}, nil
}

// Name represents the identity of a user.
type Name struct {
	// name is the name of the user, it does not include the domain.
	name string
	// domain is the part of the username after the "@".
	domain string
}

// Name returns the full username.
func (n Name) Name() string {
	if n.domain == "" || n.domain == LocalUserDomain {
		return n.name
	}
	return n.name + "@" + n.domain
}

// IsLocal indicates if the username is a local or external username.
func (n Name) IsLocal() bool {
	return n.Domain() == LocalUserDomain || n.Domain() == ""
}

// Domain returns the user domain. Users in the local database
// are from the LocalDomain. Other users are considered 'remote' users.
func (n Name) Domain() string {
	return n.domain
}

// String returns the full username.
func (n Name) String() string {
	return n.Name()
}

// IsZero return true if the struct is uninitiated.
func (n Name) IsZero() bool {
	// The empty string in an invalid user name so the struct is uninitiated if
	// it is empty.
	return n.name == "" && n.domain == ""
}

// NameFromTag generates a Name from a tag.
func NameFromTag(tag userNameTag) Name {
	return Name{
		name:   tag.Name(),
		domain: tag.Domain(),
	}
}

// IsValidName returns whether the given name is a valid user name string.
func IsValidName(name string) bool {
	return validName.MatchString(name)
}
