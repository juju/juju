// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
)

type backupsSuite struct {
	baseSuite
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) TestClient(c *gc.C) {
	facade := backups.ExposeFacade(s.client)

	c.Check(facade.Name(), gc.Equals, "Backups")
}
