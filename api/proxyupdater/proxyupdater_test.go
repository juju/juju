// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func newAPI(c *gc.C, args ...apitesting.APICall) (*int, *proxyupdater.API) {
	apiCaller := apitesting.APICallChecker(c, args...)
	api, err := proxyupdater.NewAPI(apiCaller, names.NewUnitTag("u/0"))
	c.Assert(err, gc.IsNil)
	c.Assert(api, gc.NotNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 0)

	return &apiCaller.CallCount, api
}

func (s *ProxyUpdaterSuite) TestNewAPISuccess(c *gc.C) {
	newAPI(c)
}

func (s *ProxyUpdaterSuite) TestNilAPICallerFails(c *gc.C) {
	api, err := proxyupdater.NewAPI(nil, names.NewUnitTag("u/0"))
	c.Check(api, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "caller is nil")
}

func (s *ProxyUpdaterSuite) TestNilTagFails(c *gc.C) {
	apiCaller := apitesting.APICallChecker(c)
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

	called, api := newAPI(c, apitesting.APICall{
		Facade:  "ProxyUpdater",
		Method:  "WatchForProxyConfigAndAPIHostPortChanges",
		Results: res,
	})

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

	called, api := newAPI(c, apitesting.APICall{
		Facade: "ProxyUpdater",
		Method: "ProxyConfig",
		Results: params.ProxyConfigResults{
			Results: []params.ProxyConfigResult{conf},
		},
	})

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
