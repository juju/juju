// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"gopkg.in/juju/names.v2"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func newAPI(c *gc.C, args []apitesting.CheckArgs) (*int, *proxyupdater.API) {
	var called int
	apiCaller := apitesting.CheckingAPICallerMultiArgs(c, args, &called, nil)
	api, err := proxyupdater.NewAPI(apiCaller, names.NewUnitTag("u/0"))
	c.Assert(err, gc.IsNil)
	c.Assert(api, gc.NotNil)
	c.Assert(called, gc.Equals, 0)

	return &called, api
}

func (s *ProxyUpdaterSuite) TestNewAPISuccess(c *gc.C) {
	newAPI(c, nil)
}

func (s *ProxyUpdaterSuite) TestNilAPICallerFails(c *gc.C) {
	api, err := proxyupdater.NewAPI(nil, names.NewUnitTag("u/0"))
	c.Check(api, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "caller is nil")
}

func (s *ProxyUpdaterSuite) TestNilTagFails(c *gc.C) {
	var args []apitesting.CheckArgs
	var calls int
	apiCaller := apitesting.CheckingAPICallerMultiArgs(c, args, &calls, nil)
	api, err := proxyupdater.NewAPI(apiCaller, nil)
	c.Check(api, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "tag is nil")
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *gc.C) {
	res := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{params.NotifyWatchResult{
			NotifyWatcherId: "4242",
		}},
	}

	fake := &struct {
		watcher.NotifyWatcher
	}{}
	s.PatchValue(proxyupdater.NewNotifyWatcher, func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Assert(result, gc.DeepEquals, res.Results[0])
		return fake
	})

	args := []apitesting.CheckArgs{{
		Facade:  "ProxyUpdater",
		Method:  "WatchForProxyConfigAndAPIHostPortChanges",
		Results: res,
	}}
	called, api := newAPI(c, args)

	watcher, err := api.WatchForProxyConfigAndAPIHostPortChanges()
	c.Check(*called, jc.GreaterThan, 0)
	c.Check(err, jc.ErrorIsNil)
	c.Check(watcher, gc.Equals, fake)
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *gc.C) {
	conf := params.ProxyConfigResult{
		ProxySettings: params.ProxyConfig{
			HTTP:    "http",
			HTTPS:   "https",
			FTP:     "ftp",
			NoProxy: "NoProxy",
		},
		APTProxySettings: params.ProxyConfig{
			HTTP:    "http-apt",
			HTTPS:   "https-apt",
			FTP:     "ftp-apt",
			NoProxy: "NoProxy-apt",
		},
	}
	expected := params.ProxyConfigResults{
		Results: []params.ProxyConfigResult{conf},
	}

	args := []apitesting.CheckArgs{{
		Facade:  "ProxyUpdater",
		Method:  "ProxyConfig",
		Results: expected,
	}}
	called, api := newAPI(c, args)

	proxySettings, APTProxySettings, err := api.ProxyConfig()
	c.Assert(*called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(proxySettings, jc.DeepEquals, proxy.Settings{
		Http:    "http",
		Https:   "https",
		Ftp:     "ftp",
		NoProxy: "NoProxy",
	})
	c.Check(APTProxySettings, jc.DeepEquals, proxy.Settings{
		Http:    "http-apt",
		Https:   "https-apt",
		Ftp:     "ftp-apt",
		NoProxy: "NoProxy-apt",
	})
}
