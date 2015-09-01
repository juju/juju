// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
)

// AuthenticatorForTag looks up the authenticator for the given tag.
func AuthenticatorForTag(tag string) (EntityAuthenticator, error) {
	if tag == "" {
		// If no tag is supplied we rely on macaroon authentication to provide
		// the name of the entity to be authenticated.
		// TODO (mattyw, mhilton) need a bakery
		return &MacaroonAuthenticator{}, nil
	}
	kind, err := names.TagKind(tag)
	if err != nil {
		return nil, errors.Annotate(err, "failed to determine the tag kind")
	}
	switch kind {
	case names.MachineTagKind, names.UnitTagKind:
		return &AgentAuthenticator{}, nil
	case names.UserTagKind:
		return &UserAuthenticator{}, nil
	}

	return nil, common.ErrBadRequest
}
