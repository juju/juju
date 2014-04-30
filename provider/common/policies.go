// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// SupportsUnitPlacementPolicy provides an
// implementation of SupportsUnitPlacement
// that never returns an error, and is
// intended for embedding in environs.Environ
// implementations.
type SupportsUnitPlacementPolicy struct{}

func (*SupportsUnitPlacementPolicy) SupportsUnitPlacement() error {
	return nil
}
