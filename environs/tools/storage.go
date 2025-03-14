// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/version/v2"

	"github.com/juju/juju/environs/storage"
	coretools "github.com/juju/juju/internal/tools"
)

var ErrNoTools = errors.New("no agent binaries available")

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
// If majorVersion is -1, then all tools tarballs are used.
// If store contains no such tools, it returns ErrNoMatches.
func ReadList(ctx context.Context, stor storage.StorageReader, toolsDir string, majorVersion, minorVersion int) (coretools.List, error) {
	if minorVersion >= 0 {
		logger.Debugf(ctx, "reading v%d.%d agent binaries", majorVersion, minorVersion)
	} else {
		logger.Debugf(ctx, "reading v%d.* agent binaries", majorVersion)
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
			logger.Debugf(ctx, "failed to parse version %q: %v", vers, err)
			continue
		}
		foundAnyTools = true
		// If specified major version value supplied, major version must match.
		if majorVersion >= 0 && t.Version.Major != majorVersion {
			continue
		}
		// If specified minor version value supplied, minor version must match.
		if minorVersion >= 0 && t.Version.Minor != minorVersion {
			continue
		}
		logger.Debugf(ctx, "found %s", vers)
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
