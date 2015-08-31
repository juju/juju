// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// UserIdentityProvider performs authentication for users.
type UserAuthenticator struct {
	AgentAuthenticator
}

var _ EntityAuthenticator = (*UserAuthenticator)(nil)

// TODO: MacaroonAuthenticator
// TODO: Issue a macaroon or return pre-generated macaroon -> return ErrDischareReq
//       - where should macaroons be stored? they shouldn't, except in mem (default bakery).
//       - when should they be created?
//         - root key generated on server startup. not reused among replica servers.
//         - macaroon issued on demand, reuse same root key
//       - how do we choose user tag coming in?
//         - special username? placeholder? empty username. need to return with
//           resolved entity in state so some refactoring of authenticators reqd?
// TODO: Verify macaroons -> logged in

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (u *UserAuthenticator) Authenticate(entity state.Entity, password, nonce string) error {
	if _, ok := entity.(*state.User); ok {
		return u.AgentAuthenticator.Authenticate(entity, password, nonce)
	}

	return common.ErrBadRequest
}
