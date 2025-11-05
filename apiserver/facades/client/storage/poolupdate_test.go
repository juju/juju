// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

type poolUpdateSuite struct {
}

func TestPoolUpdateSuite(t *testing.T) {
	tc.Run(t, &poolUpdateSuite{})
}

func (s *poolUpdateSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- Updating storage pools with and without usage.
- Validating attributes against the provider.
	`)
}
