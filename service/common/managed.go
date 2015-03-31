// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/version"
)

// Managed returns a path based at the directory from which init system
// files will be managed for the given OS series. The provided path
// elements, if any, are joined to that directory path.
func Managed(series string, elem ...string) (string, error) {
	dataDir, err := paths.DataDir(version.Current.Series)
	if err != nil {
		return "", errors.Trace(err)
	}
	parts := append([]string{dataDir, "init"}, elem...)

	// TODO(ericsnow) Use a renderer?
	sep := "/"
	if !strings.Contains(dataDir, "/") {
		sep = "\\"
	}
	return strings.Join(parts, sep), nil
}

// LocalManaged returns a path based at the directory from which init
// system files will be managed for the local host. The provided path
// elements, if any, are joined to that directory path.
func LocalManaged(elem ...string) (string, error) {
	return Managed(version.Current.Series, elem...)
}
