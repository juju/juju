// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type UnitAccessorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UnitAccessorSuite{})

type appGetter struct {
	exits bool
}

func (a appGetter) ApplicationExists(name string) error {
	if a.exits {
		return nil
	}
	return errors.NotFoundf("application %q", name)
}

func (s *UnitAccessorSuite) TestApplicationAgent(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}

func (s *UnitAccessorSuite) TestApplicationNotFound(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{false})
	_, err := getAuthFunc()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitAccessorSuite) TestUnitAgent(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("gitlab/0"),
	}
	getAuthFunc := unitcommon.UnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewApplicationTag("gitlab"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("gitlab/1"))
	c.Assert(ok, jc.IsFalse)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}
