// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/caas"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type CaasSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CaasSuite{})

type appGetter struct {
	exits bool
}

func (a appGetter) ApplicationExists(name string) error {
	if a.exits {
		return nil
	}
	return errors.NotFoundf("application %q", name)
}

func (s *CaasSuite) TestApplicationAgent(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := caas.CAASUnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}

func (s *CaasSuite) TestApplicationNotFound(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}
	getAuthFunc := caas.CAASUnitAccessor(auth, appGetter{false})
	_, err := getAuthFunc()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CaasSuite) TestUnitAgent(c *gc.C) {
	auth := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("gitlab/0"),
	}
	getAuthFunc := caas.CAASUnitAccessor(auth, appGetter{true})
	authFunc, err := getAuthFunc()
	c.Assert(err, jc.ErrorIsNil)
	ok := authFunc(names.NewUnitTag("gitlab/0"))
	c.Assert(ok, jc.IsTrue)
	ok = authFunc(names.NewUnitTag("gitlab/1"))
	c.Assert(ok, jc.IsFalse)
	ok = authFunc(names.NewUnitTag("mysql/0"))
	c.Assert(ok, jc.IsFalse)
}
