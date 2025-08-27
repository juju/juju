// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

type registrySuite struct{}

func TestRegistrySuite(t *testing.T) {
	tc.Run(t, registrySuite{})
}

// TestEmptyStaticProviderRegistry tests that a static provider registry with
// no providers does not result in an error or a panic.
func (registrySuite) TestEmptyStaticProviderRegistry(c *tc.C) {
	reg := StaticProviderRegistry{}
	types, err := reg.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.HasLen, 0)
}
