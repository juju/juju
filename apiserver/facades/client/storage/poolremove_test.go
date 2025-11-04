// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

type poolRemoveSuite struct {
}

func TestPoolRemoveSuite(t *testing.T) {
	tc.Run(t, &poolRemoveSuite{})
}

func (s *poolRemoveSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- Removing storage pools from a model with and without usage.
	`)
}
