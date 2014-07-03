// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/environs"
)

// PatchDataDir temporarily overrides environs.DataDir for testing purposes.
// It returns a cleanup function that you must call later to restore the
// original value.
func PatchDataDir(path string) func() {
	originalDataDir := environs.DataDir
	environs.DataDir = func() string { return path }
	return func() { environs.DataDir = originalDataDir }
}
