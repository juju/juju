// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/proxyupdater"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
}

func TestProxyUpdaterSuite(t *stdtesting.T) { tc.Run(t, &ProxyUpdaterSuite{}) }
func newAPI(c *tc.C, args ...apitesting.APICall) (*int, *proxyupdater.API) {
	apiCaller := apitesting.APICallChecker(c, args...)
	api, err := proxyupdater.NewAPI(apiCaller.APICallerFunc, names.NewUnitTag("u/0"))
	c.Assert(err, tc.IsNil)
	c.Assert(api, tc.NotNil)
	c.Assert(apiCaller.CallCount, tc.Equals, 0)

	return &apiCaller.CallCount, api
}

func (s *ProxyUpdaterSuite) TestNewAPISuccess(c *tc.C) {
	newAPI(c)
}

func (s *ProxyUpdaterSuite) TestNilAPICallerFails(c *tc.C) {
	api, err := proxyupdater.NewAPI(nil, names.NewUnitTag("u/0"))
	c.Check(api, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "caller is nil")
}

func (s *ProxyUpdaterSuite) TestNilTagFails(c *tc.C) {
	apiCaller := apitesting.APICallChecker(c)
	api, err := proxyupdater.NewAPI(apiCaller, nil)
	c.Check(api, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "tag is nil")
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *tc.C) {
	res := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "4242",
		}},
	}

	fake := &struct {
		watcher.NotifyWatcher
	}{}
	s.PatchValue(proxyupdater.NewNotifyWatcher, func(caller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Assert(result, tc.DeepEquals, res.Results[0])
		return fake
	})

	called, api := newAPI(c, apitesting.APICall{
		Facade:  "ProxyUpdater",
		Method:  "WatchForProxyConfigAndAPIHostPortChanges",
		Results: res,
	})

	watcher, err := api.WatchForProxyConfigAndAPIHostPortChanges(c.Context())
	c.Check(*called, tc.GreaterThan, 0)
	c.Check(err, tc.ErrorIsNil)
	c.Check(watcher, tc.Equals, fake)
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *tc.C) {
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
		AptMirror: "http://mirror",
	}

	called, api := newAPI(c, apitesting.APICall{
		Facade: "ProxyUpdater",
		Method: "ProxyConfig",
		Results: params.ProxyConfigResults{
			Results: []params.ProxyConfigResult{conf},
		},
	})

	config, err := api.ProxyConfig(c.Context())
	c.Assert(*called, tc.Equals, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(config.LegacyProxy, tc.DeepEquals, proxy.Settings{
		Http:    "http-legacy",
		Https:   "https-legacy",
		Ftp:     "ftp-legacy",
		NoProxy: "no-proxy-legacy",
	})
	c.Check(config.JujuProxy, tc.DeepEquals, proxy.Settings{
		Http:    "http-juju",
		Https:   "https-juju",
		Ftp:     "ftp-juju",
		NoProxy: "no-proxy-juju",
	})
	c.Check(config.APTProxy, tc.DeepEquals, proxy.Settings{
		Http:  "http-apt",
		Https: "https-apt",
		Ftp:   "ftp-apt",
	})
	c.Check(config.SnapProxy, tc.DeepEquals, proxy.Settings{
		Http:  "http-snap",
		Https: "https-snap",
	})
	c.Check(config.AptMirror, tc.Equals, "http://mirror")
}
