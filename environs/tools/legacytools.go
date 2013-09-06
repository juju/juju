// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"launchpad.net/juju-core/environs"
	coretools "launchpad.net/juju-core/tools"
)

// LegacyFindTools returns a List containing all tools with a given
// major.minor version number available in the storages, filtered by filter.
// If minorVersion = -1, then only majorVersion is considered.
// The storages are queried in order - if *any* tools are present in a storage,
// *only* tools in that storage are available for use.
// If no *available* tools have the supplied major.minor version number, or match the
// supplied filter, the function returns a *NotFoundError.
func LegacyFindTools(storages []environs.StorageReader,
	majorVersion, minorVersion int, filter coretools.Filter) (list coretools.List, err error) {

	for _, storage := range storages {
		list, err = ReadList(storage, majorVersion, minorVersion)
		if err != ErrNoTools {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	list, err = list.Match(filter)
	if err != nil {
		return nil, err
	}
	if filter.Series != "" {
		if err := checkToolsSeries(list, filter.Series); err != nil {
			return nil, err
		}
	}
	return list, err
}
