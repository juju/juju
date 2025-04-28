// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolve

// ResolveMode is represents the resolve mode of a unit.
type ResolveMode string

func (m ResolveMode) String() string {
	return string(m)
}

const (
	// ResolveModeRetryHooks indicates that the unit should retry failed hooks
	// when resolving.
	ResolveModeRetryHooks ResolveMode = "retry-hooks"

	// ResolveModeNoHooks indicates that the unit should not retry failed hooks
	// when resolving.
	ResolveModeNoHooks ResolveMode = "no-hooks"
)
