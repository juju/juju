// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
)

// CreateLocalTestStorage returns the listener, which needs to be closed, and
// the storage that is backed by a directory created in the running test's temp
// directory.
func CreateLocalTestStorage(c *tc.C) (closer io.Closer, stor storage.Storage, dataDir string) {
	dataDir = c.MkDir()
	underlying, err := filestorage.NewFileStorageWriter(dataDir)
	c.Assert(err, jc.ErrorIsNil)
	return nopCloser{}, underlying, dataDir
}

type nopCloser struct{}

func (nopCloser) Close() error {
	return nil
}
