// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

// ProviderCallContext exposes useful capabilities when making calls
// to an underlying cloud substrate.
type ProviderCallContext interface {

	// InvalidateCredentialCallback provides means
	// of a invalidating the credential that is used to make a call.
	InvalidateCredentialCallback() error
}
