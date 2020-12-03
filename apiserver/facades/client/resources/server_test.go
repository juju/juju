// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&FacadeSuite{})

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *gc.C) {
	_, err := NewResourcesAPI(s.data, func(chID CharmID) (NewCharmRepository, error) { return &stubFactory{}, nil })
	c.Check(err, jc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingDataStore(c *gc.C) {
	_, err := NewResourcesAPI(nil, func(chID CharmID) (NewCharmRepository, error) { return &stubFactory{}, nil })
	c.Check(err, gc.ErrorMatches, `missing data backend`)
}

func (s *FacadeSuite) TestNewFacadeMissingCSClientFactory(c *gc.C) {
	_, err := NewResourcesAPI(s.data, nil)
	c.Check(err, gc.ErrorMatches, `missing factory for new repository`)
}
