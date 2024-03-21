// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

// Key represents the natural key of a cloud credential.
type Key struct {
	// Cloud is the cloud name that the credential applies to. Key is valid when
	// this value is set.
	Cloud string

	// Owner is the owner of the credential. Key is valid when this value is set.
	Owner string

	// Name is the name of the credential. Is is valid when this value is set.
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
		Owner: tag.Owner().Name(),
		Name:  tag.Name(),
	}
}

// IsZero returns true if the [Key] struct is it's zero value with no values set.
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
		return fmt.Errorf("%w cloud cannot be empty", errors.NotValid)
	}
	if k.Name == "" {
		return fmt.Errorf("%w name cannot be empty", errors.NotValid)
	}
	if k.Owner == "" {
		return fmt.Errorf("%w owner cannot be empty", errors.NotValid)
	}
	return nil
}
