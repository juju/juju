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
	DefaultToolPrefix = "tools/juju-"
	NewToolPrefix     = "tools/releases/juju-"
	toolSuffix        = ".tgz"
)

var toolPrefix string = DefaultToolPrefix

// SetToolPrefix changes the prefix used to compose the tools tarball file name.
func SetToolPrefix(prefix string) func() {
	originalPrefix := toolPrefix
	toolPrefix = prefix
	return func() {
		toolPrefix = originalPrefix
	}
}

// StorageName returns the name that is used to store and retrieve the
// given version of the juju tools.
func StorageName(vers version.Binary) string {
	return toolPrefix + vers.String() + toolSuffix
}

// ReadList returns a List of the tools in store with the given major.minor version.
// If minorVersion = -1, then only majorVersion is considered.
// If store contains no such tools, it returns ErrNoMatches.
func ReadList(stor storage.StorageReader, majorVersion, minorVersion int) (coretools.List, error) {
	origPrefix := toolPrefix
	defer func() {
		SetToolPrefix(origPrefix)
	}()
	toolsList, err := internalReadList(stor, majorVersion, minorVersion)
	if err == ErrNoTools {
		SetToolPrefix(NewToolPrefix)
		toolsList, err = internalReadList(stor, majorVersion, minorVersion)
	}
	return toolsList, err
}

func internalReadList(stor storage.StorageReader, majorVersion, minorVersion int) (coretools.List, error) {
	if minorVersion >= 0 {
		logger.Debugf("reading v%d.%d tools", majorVersion, minorVersion)
	} else {
		logger.Debugf("reading v%d.* tools", majorVersion)
	}
	names, err := storage.DefaultList(stor, toolPrefix)
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
