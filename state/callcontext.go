// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/environs/context"

func CallContext(st *State) context.ProviderCallContext {
	callCtx := context.NewCloudCallContext()
	callCtx.InvalidateCredentialFunc = st.InvalidateModelCredential
	return callCtx
}
