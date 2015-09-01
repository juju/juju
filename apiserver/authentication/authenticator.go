// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/names"
)

// FindEntityAuthenticator looks up the authenticator for the entity identified tag.
// TODO: replace "entity" with AuthTag string to dispatch to appropriate authenticator.
func FindEntityAuthenticator(tag string) (EntityAuthenticator, error) {
	kind, err := names.TagKind(tag)
	if err != nil {
		return nil, err
	}
	switch kind {
	case names.MachineTagKind, names.UnitTagKind:
		return &AgentAuthenticator{}, nil
	case names.UserTagKind:
		if tag == "" {
			// TODO need a bakery
			return &MacaroonAuthenticator{}, nil
		}
		return &UserAuthenticator{}, nil
	}

	return nil, common.ErrBadRequest
}
