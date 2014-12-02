// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/juju/arch"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var ErrNoTools = errors.New("no tools available")

const (
	toolPrefix = "tools/%s/juju-"
	toolSuffix = ".tgz"
)

// StorageName returns the name that is used to store and retrieve the
// given version of the juju tools.
func StorageName(vers version.Binary, stream string) string {
	return storagePrefix(stream) + vers.String() + toolSuffix
}

func storagePrefix(stream string) string {
	return fmt.Sprintf(toolPrefix, stream)
}

// ReadList returns a List of the tools in store with the given major.minor version.
// If minorVersion = -1, then only majorVersion is considered.
// If store contains no such tools, it returns ErrNoMatches.
func ReadList(stor storage.StorageReader, toolsDir string, majorVersion, minorVersion int) (coretools.List, error) {
	if minorVersion >= 0 {
		logger.Debugf("reading v%d.%d tools", majorVersion, minorVersion)
	} else {
		logger.Debugf("reading v%d.* tools", majorVersion)
	}
	storagePrefix := storagePrefix(toolsDir)
	names, err := storage.List(stor, storagePrefix)
	if err != nil {
		return nil, err
	}
	var list coretools.List
	var foundAnyTools bool
	for _, name := range names {
		name = filepath.ToSlash(name)
		if !strings.HasPrefix(name, storagePrefix) || !strings.HasSuffix(name, toolSuffix) {
			continue
		}
		var t coretools.Tools
		vers := name[len(storagePrefix) : len(name)-len(toolSuffix)]
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
		// Older versions of Juju only know about ppc64, so add metadata for that arch.
		if t.Version.Arch == arch.PPC64EL {
			legacyPPC64Tools := t
			legacyPPC64Tools.Version.Arch = arch.LEGACY_PPC64
			list = append(list, &legacyPPC64Tools)
		}
	}
	if len(list) == 0 {
		if foundAnyTools {
			return nil, coretools.ErrNoMatches
		}
		return nil, ErrNoTools
	}
	return list, nil
}
