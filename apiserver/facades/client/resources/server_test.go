// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"github.com/juju/charm/v11"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/resources"
)

var _ = gc.Suite(&FacadeSuite{})

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *gc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(s.backend, func(*charm.URL) (resources.NewCharmRepository, error) { return s.factory, nil })
	c.Check(err, jc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingDataStore(c *gc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(nil, func(*charm.URL) (resources.NewCharmRepository, error) { return s.factory, nil })
	c.Check(err, gc.ErrorMatches, `missing data backend`)
}

func (s *FacadeSuite) TestNewFacadeMissingCSClientFactory(c *gc.C) {
	defer s.setUpTest(c).Finish()
	_, err := resources.NewResourcesAPI(s.backend, nil)
	c.Check(err, gc.ErrorMatches, `missing factory for new repository`)
}
