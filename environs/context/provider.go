// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import "context"

// ProviderCallContext exposes useful capabilities when making calls
// to an underlying cloud substrate.
type ProviderCallContext interface {
	context.Context

	// InvalidateCredential provides means to invalidate a credential
	// that is used to make a call.
	InvalidateCredential(string) error
}
