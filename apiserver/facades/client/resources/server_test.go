// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/resources"
)

var _ = gc.Suite(&FacadeSuite{})

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *gc.C) {
	_, err := resources.NewResourcesAPI(s.data, s.newCSClient)
	c.Check(err, jc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingDataStore(c *gc.C) {
	_, err := resources.NewResourcesAPI(nil, s.newCSClient)
	c.Check(err, gc.ErrorMatches, `missing data store`)
}

func (s *FacadeSuite) TestNewFacadeMissingCSClientFactory(c *gc.C) {
	_, err := resources.NewResourcesAPI(s.data, nil)
	c.Check(err, gc.ErrorMatches, `missing factory for new charm store clients`)
}
