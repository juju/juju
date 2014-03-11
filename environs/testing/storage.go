// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/state"
)

// CreateLocalTestStorage returns the listener, which needs to be closed, and
// the storage that is backed by a directory created in the running test's temp
// directory.
func CreateLocalTestStorage(c *gc.C) (closer io.Closer, stor storage.Storage, dataDir string) {
	dataDir = c.MkDir()
	underlying, err := filestorage.NewFileStorageWriter(dataDir)
	c.Assert(err, gc.IsNil)
	listener, err := httpstorage.Serve("localhost:0", underlying)
	c.Assert(err, gc.IsNil)
	stor = httpstorage.Client(listener.Addr().String())
	closer = listener
	return
}

// GetEnvironStorage creates an Environ from the config in state and
// returns its storage interface.
func GetEnvironStorage(st *state.State) (storage.Storage, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}
