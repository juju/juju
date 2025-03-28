// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"fmt"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// Key represents the natural key of a cloud credential.
type Key struct {
	// Cloud is the cloud name that the credential applies to. Key is valid when
	// this value is set.
	Cloud string

	// Owner is the owner of the credential. Key is valid when this value is set.
	Owner user.Name

	// Name is the name of the credential. It is valid when this value is set.
	Name string
}

// KeyFromTag provides a utility for converting a CloudCredentialTag into a Key
// struct. If the tags IsZero() returns true then a zero value Key struct is
// returned.
func KeyFromTag(tag names.CloudCredentialTag) Key {
	if tag.IsZero() {
		return Key{}
	}

	return Key{
		Cloud: tag.Cloud().Id(),
		Owner: user.NameFromTag(tag.Owner()),
		Name:  tag.Name(),
	}
}

// IsZero returns true if the [Key] struct is its zero value with no values set.
func (k Key) IsZero() bool {
	return k == Key{}
}

// String implements the stringer interface.
func (k Key) String() string {
	return fmt.Sprintf("%s/%s/%s", k.Cloud, k.Owner, k.Name)
}

// Tag will convert this Key struct to a juju names CloudCredentialTag. Errors in
// parsing of the tag will be returned. If the Key struct is it's zero value then
// a zero value Tag will be returned.
func (k Key) Tag() (names.CloudCredentialTag, error) {
	if k.IsZero() {
		return names.CloudCredentialTag{}, nil
	}
	return names.ParseCloudCredentialTag(
		fmt.Sprintf("%s-%s_%s_%s", names.CloudCredentialTagKind, k.Cloud, k.Owner, k.Name),
	)
}

// Validate is responsible for checking all of the fields of Key are in a set
// state that is valid for use. You can also use IsZero() to test if the Key is
// currently set to it's zero value.
func (k Key) Validate() error {
	if k.Cloud == "" {
		return errors.Errorf("cloud cannot be empty").Add(coreerrors.NotValid)
	}
	if k.Name == "" {
		return errors.Errorf("name cannot be empty").Add(coreerrors.NotValid)
	}
	if k.Owner.IsZero() {
		return errors.Errorf("owner cannot be empty").Add(coreerrors.NotValid)
	}
	return nil
}

// UUID represents a unique id within the juju controller for a cloud credential.
type UUID string

// NewUUID generates a new credential [UUID]
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), errors.Errorf("creating new credential id: %w", err)
	}
	return UUID(uuid.String()), nil
}

// String implements the stringer interface returning a string representation of
// the credential UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the uuid. If the [UUID] is invalid an
// error satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return errors.Errorf("credential uuid cannot be empty").Add(coreerrors.NotValid)
	}

	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("credential uuid %q %w", u, coreerrors.NotValid)
	}
	return nil
}
