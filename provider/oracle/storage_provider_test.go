// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/juju/environs"
	gc "gopkg.in/check.v1"
)

type storageProviderSuite struct{}

var _ = gc.Suite(&storageProviderSuite{})

func (s storageProviderSuite) NewStorageProvider(c *gc.C) environs.Provider {
	return nil
}
