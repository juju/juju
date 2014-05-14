// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"errors"
	"strings"

	"launchpad.net/juju-core/environs/storage"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

var ErrNoTools = errors.New("no tools available")

const (
	toolPrefix = "tools/releases/juju-"
	toolSuffix = ".tgz"
)

// StorageName returns the name that is used to store and retrieve the
// given version of the juju tools.
func StorageName(vers version.Binary) string {
	return toolPrefix + vers.String() + toolSuffix
}

// ReadList returns a List of the tools in store with the given major.minor version.
// If minorVersion = -1, then only majorVersion is considered.
// If store contains no such tools, it returns ErrNoMatches.
func ReadList(stor storage.StorageReader, majorVersion, minorVersion int) (coretools.List, error) {
	if minorVersion >= 0 {
		logger.Debugf("reading v%d.%d tools", majorVersion, minorVersion)
	} else {
		logger.Debugf("reading v%d.* tools", majorVersion)
	}
	names, err := storage.List(stor, toolPrefix)
	if err != nil {
		return nil, err
	}
	var list coretools.List
	var foundAnyTools bool
	for _, name := range names {
		if !strings.HasPrefix(name, toolPrefix) || !strings.HasSuffix(name, toolSuffix) {
			continue
		}
		var t coretools.Tools
		vers := name[len(toolPrefix) : len(name)-len(toolSuffix)]
		if t.Version, err = version.ParseBinary(vers); err != nil {
			logger.Debugf("failed to parse version %q: %v", vers, err)
			continue
		}
		foundAnyTools = true
		// Major version must match specified value.
		if t.Version.Major != majorVersion {
			continue
		}
		// If specified minor version value supplied, minor version must match.
		if minorVersion >= 0 && t.Version.Minor != minorVersion {
			continue
		}
		logger.Debugf("found %s", vers)
		if t.URL, err = stor.URL(name); err != nil {
			return nil, err
		}
		list = append(list, &t)
	}
	if len(list) == 0 {
		if foundAnyTools {
			return nil, coretools.ErrNoMatches
		}
		return nil, ErrNoTools
	}
	return list, nil
}
