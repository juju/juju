// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/cloudconfig/instancecfg"

// PatchDataDir temporarily overrides environs.DataDir for testing purposes.
// It returns a cleanup function that you must call later to restore the
// original value.
func PatchDataDir(path string) func() {
	originalDataDir := instancecfg.DataDir
	instancecfg.DataDir = path
	return func() { instancecfg.DataDir = originalDataDir }
}
