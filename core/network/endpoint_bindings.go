// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// MapBindingsWithSpaceNames maps a collection of endpoint bindings (i.e. a map
// of endpoint names to space IDs) to the endpoint bindings with the corresponding
// space names, using the provided SpaceInfos.
func MapBindingsWithSpaceNames(bindingsMap map[string]string, lookup SpaceInfos) (map[string]string, error) {
	// Handle the fact that space name lookup can be nil or empty.
	if len(bindingsMap) > 0 && len(lookup) == 0 {
		return nil, errors.Errorf("empty space lookup").Add(coreerrors.NotValid)
	}

	retVal := make(map[string]string, len(bindingsMap))

	// Assume that bindings is always in space id format due to
	// Bindings constructor.
	for k, v := range bindingsMap {
		spaceInfo := lookup.GetByID(v)
		if spaceInfo == nil {
			return nil, errors.Errorf("space with ID %q", v).Add(coreerrors.NotFound)
		}
		retVal[k] = string(spaceInfo.Name)
	}
	return retVal, nil
}
