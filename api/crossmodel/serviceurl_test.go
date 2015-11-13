// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/testing"
)

type serviceURLSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&serviceURLSuite{})

func (s *serviceURLSuite) TestUnsupportedURL(c *gc.C) {
	f, err := crossmodel.NewServiceAPIFactory(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = f.ServiceDirectory("vendor-test:/u/me/service")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *serviceURLSuite) TestLocalURL(c *gc.C) {
	caller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			return errors.New("error")
		})
	f, err := crossmodel.NewServiceAPIFactory(caller)
	c.Assert(err, jc.ErrorIsNil)
	api, err := f.ServiceDirectory("local:/u/me/service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crossmodel.IsServiceDirecotryAPIFacade(api), jc.IsTrue)
}
