// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/version"
)

// ManagedDir returns the path to the directory from which init system
// files will be managed for the given OS series.
func ManagedDir(series string) (string, error) {
	dataDir, err := paths.DataDir(version.Current.Series)
	if err != nil {
		return "", errors.Trace(err)
	}
	// TODO(ericsnow) Use a renderer?
	return filepath.Join(dataDir, "init"), nil
}

// ManagedDir returns the path to the directory from which init system
// files will be managed for the local host.
func LocalManagedDir() (string, error) {
	return ManagedDir(version.Current.Series)
}
