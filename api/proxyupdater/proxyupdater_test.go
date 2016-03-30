// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/proxyupdater"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite

	called    int
	apiCaller base.APICallCloser
	api       *proxyupdater.API
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) init(c *gc.C, args []apitesting.CheckArgs, err error) {
	s.called = 0
	s.apiCaller = apitesting.CheckingAPICallerMultiArgs(c, args, &s.called, err)
	s.api = proxyupdater.NewAPI(s.apiCaller)
	c.Check(s.api, gc.NotNil)
	c.Check(s.called, gc.Equals, 0)
}

func (s *ProxyUpdaterSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	apiCaller := apitesting.CheckingAPICaller(c, nil, &called, nil)
	api := proxyupdater.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)
}

func (s *ProxyUpdaterSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { proxyupdater.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *gc.C) {
	args := []apitesting.CheckArgs{{
		Facade:  "ProxyUpdater",
		Method:  "WatchForProxyConfigAndAPIHostPortChanges",
		Args:    nil,
		Results: nil,
	}}

	s.init(c, args, nil)
	_, err := s.api.WatchForProxyConfigAndAPIHostPortChanges()
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *gc.C) {
	args := []apitesting.CheckArgs{{
		Facade:  "ProxyUpdater",
		Method:  "ProxyConfig",
		Args:    nil,
		Results: nil,
	}}

	s.init(c, args, nil)
	_, err := s.api.ProxyConfig()
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
}
