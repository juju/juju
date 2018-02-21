// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
)

// CloudCredential is a convenience method to create state.Credential to be used in unit tests.
func CloudCredential(authType cloud.AuthType, attrs map[string]string) state.Credential {
	c := state.Credential{}
	c.AuthType = string(authType)
	c.Attributes = attrs
	return c
}

// NewEmptyCredential is a convenience method to create an empty state.Credential
// with a cloud.EmptyAuthType as auth type to be used in unit tests.
func NewEmptyCredential() state.Credential {
	c := state.Credential{}
	c.AuthType = string(cloud.EmptyAuthType)
	return c
}
