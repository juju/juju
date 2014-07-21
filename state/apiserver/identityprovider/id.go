// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityprovider

import (
	"fmt"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// IdentityProvider is the interface all identity providers need to implement
// to authenticate juju entities.
type IdentityProvider interface {
	Login(st *state.State, tag names.Tag, password, nonce string) error
}

// LookupProvider looks up the identity provider for the entity identified tag.
func LookupProvider(tag names.Tag) (IdentityProvider, error) {
	switch tag.Kind() {
	case names.MachineTagKind, names.UnitTagKind:
		return &AgentIdentityProvider{}, nil
	case names.UserTagKind:
		return &UserIdentityProvider{}, nil
	}
	return nil, fmt.Errorf("Tag type '%s' does not have an identity provider", tag.Kind())
}
