// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// ProviderType is a unique identification type to represent individual storage
// providers within a controller by common value.
type ProviderType string

// String returns the string representation of [ProviderType]. String implements
// the [fmt.Stringer] interface.
func (p ProviderType) String() string {
	return string(p)
}
