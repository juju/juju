// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/crossmodel"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type serviceURLSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&serviceURLSuite{})

func (s *serviceURLSuite) TestUnsupportedURL(c *gc.C) {
	f, err := crossmodel.NewServiceAPIFactory(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = f.ServiceOffers("unsupported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *serviceURLSuite) TestLocalURL(c *gc.C) {
	var st *state.State
	f, err := crossmodel.NewServiceAPIFactory(
		func() jujucrossmodel.ServiceDirectory { return state.NewServiceDirectory(st) },
	)
	c.Assert(err, jc.ErrorIsNil)
	api, err := f.ServiceOffers("local")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := api.(crossmodel.ServiceOffersAPI)
	c.Assert(ok, jc.IsTrue)
}
