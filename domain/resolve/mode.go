// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolve

type ResolveMode string

const (
	// ResolveModeRetryHooks indicates that the unit should retry failed hooks
	// when resolving.
	ResolveModeRetryHooks ResolveMode = "retry-hooks"

	// ResolveModeNoHooks indicates that the unit should not retry failed hooks
	// when resolving.
	ResolveModeNoHooks ResolveMode = "no-hooks"
)
