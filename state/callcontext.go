// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/environs/context"

func CreateCallContext(st *State) context.ProviderCallContext {
	callCtx := context.NewCloudCallContext()
	callCtx.InvalidateCredentialF = func(reason string) error {
		return st.InvalidateModelCredential(reason)
	}
	return callCtx
}
