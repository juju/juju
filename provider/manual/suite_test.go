// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/provider/manual"
)

func Test(t *testing.T) {
	// Prevent any use of ssh for storage.
	*manual.NewSSHStorage = func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, nil
	}
	gc.TestingT(t)
}
