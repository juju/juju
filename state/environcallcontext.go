// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
)

// callContext implements context.ProviderCallContext and is used
// to make calls to substrate from within state package.
type callContext struct {
	internal *State
}

// InvalidateCredentialCallback implements context.InvalidateCredentialCallback
func (*callContext) InvalidateCredentialCallback() error {
	return errors.NotImplementedf("InvalidateCredentialCallback")
}
