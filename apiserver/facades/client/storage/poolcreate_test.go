// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

type poolCreateSuite struct {
}

func TestPoolCreateSuite(t *testing.T) {
	tc.Run(t, &poolCreateSuite{})
}

func (s *poolCreateSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- Creating storage pools of all shapes and sizes
	`)
}
