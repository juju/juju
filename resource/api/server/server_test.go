// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/api/server"
)

var _ = gc.Suite(&FacadeSuite{})

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *gc.C) {
	_, err := server.NewFacade(s.data, s.newCSClient)

	c.Check(err, jc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingDataStore(c *gc.C) {
	_, err := server.NewFacade(nil, s.newCSClient)

	c.Check(err, gc.ErrorMatches, `missing data store`)
}

func (s *FacadeSuite) TestNewFacadeMissingCSClientFactory(c *gc.C) {
	_, err := server.NewFacade(s.data, nil)

	c.Check(err, gc.ErrorMatches, `missing factory for new charm store clients`)
}
