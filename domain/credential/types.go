// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
)

// ID represents the id of a cloud credential.
type ID struct {
	// Cloud is the cloud name that the credential applies to. Id is valid when
	// this value is set.
	Cloud string

	// Owner is the owner of the credential. Id is valid when this value is set.
	Owner string

	// Name is the name of the credential. Is is valid when this value is set.
	Name string
}

// IdFromTag provides a utility for converting a CloudCredentialTag into a Id
// struct. If the tags IsZero() returns true then a zero value Id struct is
// returned.
func IdFromTag(tag names.CloudCredentialTag) ID {
	if tag.IsZero() {
		return ID{}
	}

	return ID{
		Cloud: tag.Cloud().Id(),
		Owner: tag.Owner().Name(),
		Name:  tag.Name(),
	}
}

// IsZero returns true if the ID struct is it's zero value with no values set.
func (i ID) IsZero() bool {
	return i == ID{}
}

// String implements the stringer interface.
func (i ID) String() string {
	return fmt.Sprintf("%s/%s/%s", i.Cloud, i.Owner, i.Name)
}

// Tag will convert this Id struct to a juju names CloudCredentialTag. Errors in
// parsing of the tag will be returned. If the Id struct is it's zero value then
// a zero value Tag will be returned.
func (i ID) Tag() (names.CloudCredentialTag, error) {
	if i.IsZero() {
		return names.CloudCredentialTag{}, nil
	}
	return names.ParseCloudCredentialTag(
		fmt.Sprintf("%s-%s_%s_%s", names.CloudCredentialTagKind, i.Cloud, i.Owner, i.Name),
	)
}

// Validate is responsible for checking all of the fields of Id are in a set
// state that is valid for use. You can also use IsZero() to test if the Id is
// currently set to it's zero value.
func (i ID) Validate() error {
	if i.Cloud == "" {
		return fmt.Errorf("%w cloud cannot be empty", errors.NotValid)
	}
	if i.Name == "" {
		return fmt.Errorf("%w name cannot be empty", errors.NotValid)
	}
	if i.Owner == "" {
		return fmt.Errorf("%w owner cannot be empty", errors.NotValid)
	}
	return nil
}

// CloudCredentialInfo represents a credential.
type CloudCredentialInfo struct {
	// AuthType is the credential auth type.
	AuthType string

	// Attributes are the credential attributes.
	Attributes map[string]string

	// Revoked is true if the credential has been revoked.
	Revoked bool

	// Label is optionally set to describe the credentials to a user.
	Label string

	// Invalid is true if the credential is invalid.
	Invalid bool

	// InvalidReason contains the reason why a credential was flagged as invalid.
	// It is expected that this string will be empty when a credential is valid.
	InvalidReason string
}

// CloudCredentialResult represents a credential and the cloud it belongs to.
type CloudCredentialResult struct {
	CloudCredentialInfo

	// CloudName is the cloud the credential belongs to.
	CloudName string
}
