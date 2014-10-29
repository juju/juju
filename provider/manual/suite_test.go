// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/provider/manual"
)

func Test(t *testing.T) {
	// Prevent any use of ssh for storage.
	*manual.NewSSHStorage = func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, nil
	}
	gc.TestingT(t)
}
