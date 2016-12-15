// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type applicationURLSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&applicationURLSuite{})

func (s *applicationURLSuite) TestUnsupportedURL(c *gc.C) {
	f, err := crossmodel.NewServiceAPIFactory(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = f.ApplicationOffers("unsupported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *applicationURLSuite) TestLocalURL(c *gc.C) {
	var st *state.State
	f, err := crossmodel.NewServiceAPIFactory(
		func() jujucrossmodel.ApplicationDirectory { return state.NewApplicationDirectory(st) },
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	api, err := f.ApplicationOffers("local")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := api.(crossmodel.ApplicationOffersAPI)
	c.Assert(ok, jc.IsTrue)
}

type closer struct {
	called bool
}

func (c *closer) Close() error {
	c.called = true
	return nil
}

func (s *applicationURLSuite) TestStop(c *gc.C) {
	var st *state.State
	closer := &closer{}
	f, err := crossmodel.NewServiceAPIFactory(
		func() jujucrossmodel.ApplicationDirectory { return state.NewApplicationDirectory(st) },
		closer,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = f.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(closer.called, jc.IsTrue)
}
