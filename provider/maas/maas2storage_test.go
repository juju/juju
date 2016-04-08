// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	gc "gopkg.in/check.v1"
)

type maas2StorageSuite struct {
	maas2EnvironSuite
}

var _ = gc.Suite(&maas2StorageSuite{})

func (s *maas2EnvironSuite) TestGetNoSuchFile(c *gc.C) {
}

func (s *maas2EnvironSuite) TestGetReadFails(c *gc.C) {
}

func (s *maas2EnvironSuite) TestGetSuccess(c *gc.C) {
}

func (s *maas2EnvironSuite) TestListError(c *gc.C) {
}

func (s *maas2EnvironSuite) TestListSuccess(c *gc.C) {
}
