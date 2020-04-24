// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"github.com/juju/names/v4"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/testing"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func newAPI(c *gc.C, version int, args ...apitesting.APICall) (*int, *proxyupdater.API) {
	apiCaller := apitesting.APICallChecker(c, args...)
	api, err := proxyupdater.NewAPI(
		apitesting.BestVersionCaller{
			APICallerFunc: apiCaller.APICallerFunc,
			BestVersion:   version,
		}, names.NewUnitTag("u/0"))
	c.Assert(err, gc.IsNil)
	c.Assert(api, gc.NotNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 0)

	return &apiCaller.CallCount, api
}

func (s *ProxyUpdaterSuite) TestNewAPISuccess(c *gc.C) {
	newAPI(c, 2)
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
		Results: []params.NotifyWatchResult{{
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

	called, api := newAPI(c, 2, apitesting.APICall{
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
		LegacyProxySettings: params.ProxyConfig{
			HTTP:    "http-legacy",
			HTTPS:   "https-legacy",
			FTP:     "ftp-legacy",
			NoProxy: "no-proxy-legacy",
		},
		JujuProxySettings: params.ProxyConfig{
			HTTP:    "http-juju",
			HTTPS:   "https-juju",
			FTP:     "ftp-juju",
			NoProxy: "no-proxy-juju",
		},
		APTProxySettings: params.ProxyConfig{
			HTTP:  "http-apt",
			HTTPS: "https-apt",
			FTP:   "ftp-apt",
		},
		SnapProxySettings: params.ProxyConfig{
			HTTP:  "http-snap",
			HTTPS: "https-snap",
		},
	}

	called, api := newAPI(c, 2, apitesting.APICall{
		Facade: "ProxyUpdater",
		Method: "ProxyConfig",
		Results: params.ProxyConfigResults{
			Results: []params.ProxyConfigResult{conf},
		},
	})

	config, err := api.ProxyConfig()
	c.Assert(*called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config.LegacyProxy, jc.DeepEquals, proxy.Settings{
		Http:    "http-legacy",
		Https:   "https-legacy",
		Ftp:     "ftp-legacy",
		NoProxy: "no-proxy-legacy",
	})
	c.Check(config.JujuProxy, jc.DeepEquals, proxy.Settings{
		Http:    "http-juju",
		Https:   "https-juju",
		Ftp:     "ftp-juju",
		NoProxy: "no-proxy-juju",
	})
	c.Check(config.APTProxy, jc.DeepEquals, proxy.Settings{
		Http:  "http-apt",
		Https: "https-apt",
		Ftp:   "ftp-apt",
	})
	c.Check(config.SnapProxy, jc.DeepEquals, proxy.Settings{
		Http:  "http-snap",
		Https: "https-snap",
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigV1(c *gc.C) {
	conf := params.ProxyConfigResultV1{
		ProxySettings: params.ProxyConfig{
			HTTP:    "http-legacy",
			HTTPS:   "https-legacy",
			FTP:     "ftp-legacy",
			NoProxy: "no-proxy-legacy",
		},
		APTProxySettings: params.ProxyConfig{
			HTTP:  "http-apt",
			HTTPS: "https-apt",
			FTP:   "ftp-apt",
		},
	}

	called, api := newAPI(c, 1, apitesting.APICall{
		Facade: "ProxyUpdater",
		Method: "ProxyConfig",
		Results: params.ProxyConfigResultsV1{
			Results: []params.ProxyConfigResultV1{conf},
		},
	})

	config, err := api.ProxyConfig()
	c.Assert(*called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config.LegacyProxy, jc.DeepEquals, proxy.Settings{
		Http:    "http-legacy",
		Https:   "https-legacy",
		Ftp:     "ftp-legacy",
		NoProxy: "no-proxy-legacy",
	})
	c.Check(config.JujuProxy, jc.DeepEquals, proxy.Settings{})
	c.Check(config.APTProxy, jc.DeepEquals, proxy.Settings{
		Http:  "http-apt",
		Https: "https-apt",
		Ftp:   "ftp-apt",
	})
	c.Check(config.SnapProxy, jc.DeepEquals, proxy.Settings{})
}
