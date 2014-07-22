// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// FindEntityAuthenticator looks up the authenticator for the entity identified tag.
func FindEntityAuthenticator(entity state.Entity) (TagAuthenticator, error) {
	kind := entity.Tag().Kind()
	switch kind {
	case names.MachineTagKind, names.UnitTagKind:
		return &AgentAuthenticator{}, nil
	case names.UserTagKind:
		return &UserAuthenticator{}, nil
	}
	return nil, errors.Errorf("entity with tag type '%s' does not have an authenticator", kind)
}
