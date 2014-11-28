// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/httpstorage"
	"github.com/juju/juju/environs/storage"
)

// CreateLocalTestStorage returns the listener, which needs to be closed, and
// the storage that is backed by a directory created in the running test's temp
// directory.
func CreateLocalTestStorage(c *gc.C) (closer io.Closer, stor storage.Storage, dataDir string) {
	dataDir = c.MkDir()
	underlying, err := filestorage.NewFileStorageWriter(dataDir)
	c.Assert(err, jc.ErrorIsNil)
	listener, err := httpstorage.Serve("localhost:0", underlying)
	c.Assert(err, jc.ErrorIsNil)
	stor = httpstorage.Client(listener.Addr().String())
	closer = listener
	return
}
