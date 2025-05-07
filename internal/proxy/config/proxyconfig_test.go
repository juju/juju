// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"net/http"

	"github.com/juju/proxy"
	"github.com/juju/tc"

	proxyconfig "github.com/juju/juju/internal/proxy/config"
)

type Suite struct{}

var _ = tc.Suite(&Suite{})

func checkProxy(c *tc.C, settings proxy.Settings, requestURL, expectedURL string) {
	pc := proxyconfig.ProxyConfig{}
	c.Assert(pc.Set(settings), tc.ErrorIsNil)
	req, err := http.NewRequest("GET", requestURL, nil)
	c.Assert(err, tc.ErrorIsNil)
	proxyURL, err := pc.GetProxy(req)
	c.Assert(err, tc.ErrorIsNil)
	if expectedURL == "" {
		c.Check(proxyURL, tc.IsNil)
	} else {
		c.Assert(proxyURL, tc.Not(tc.IsNil))
		c.Check(proxyURL.String(), tc.Equals, expectedURL)
	}
}

var (
	normal = proxy.Settings{
		Http:    "http://http.proxy",
		Https:   "https://https.proxy",
		NoProxy: ".foo.com,bar.com,10.0.0.1:3333,192.168.0.0/16",
	}
	noProxy = proxy.Settings{
		Http:    "http://http.proxy",
		NoProxy: "*",
	}
	ipv6Proxy = proxy.Settings{
		Http:  "2001:db8:85a3::8a2e:370:7334",
		Https: "[2001:db8:85a3::8a2e:370:7334]:80",
	}
)

func (s *Suite) TestGetProxy(c *tc.C) {
	checkProxy(c, normal, "https://perfect.crime", "https://https.proxy")
	checkProxy(c, normal, "http://decemberists.com", "http://http.proxy")
	checkProxy(c, normal, "http://[2001:db8:85a3::8a2e:370:7334]", "http://http.proxy")
	checkProxy(c, ipv6Proxy, "http://[2001:db8:85a3::8a2e:370:7334]", "http://2001:db8:85a3::8a2e:370:7334")
	checkProxy(c, proxy.Settings{}, "https://sufjan.stevens", "")
	checkProxy(c, normal, "http://adz.foo.com:80", "")
	checkProxy(c, normal, "http://adz.bar.com", "")
	checkProxy(c, normal, "http://localhost", "")
	checkProxy(c, normal, "http://127.0.0.1", "")
	checkProxy(c, normal, "http://[::1]:8000", "")
	checkProxy(c, normal, "http://10.0.0.1", "")
	checkProxy(c, normal, "http://10.0.0.1:1996", "")
	checkProxy(c, normal, "http://10.23.45.67:80", "http://http.proxy")
	checkProxy(c, noProxy, "http://decemberists.com", "")
	checkProxy(c, proxy.Settings{Http: "grizzly.bear"}, "veckatimest.com", "http://grizzly.bear")
	checkProxy(c, normal, "http://192.168.30.40:80", "")
}

func (s *Suite) TestSetBadUrl(c *tc.C) {
	pc := proxyconfig.ProxyConfig{}
	err := pc.Set(proxy.Settings{
		Https: "http://badurl%gg",
	})
	c.Assert(err, tc.ErrorMatches, `https proxy: invalid proxy address "http://badurl%gg": .*$`)
	err = pc.Set(proxy.Settings{
		Http: "http://badurl%gg",
	})
	c.Assert(err, tc.ErrorMatches, `http proxy: invalid proxy address "http://badurl%gg": .*$`)
}

func (s *Suite) TestInstallError(c *tc.C) {
	type fakeRoundTripper struct {
		http.RoundTripper
	}
	// PatchValue doesn't work for this for some reflection/type
	// reason.
	oldTransport := http.DefaultTransport
	http.DefaultTransport = &fakeRoundTripper{}
	defer func() {
		http.DefaultTransport = oldTransport
	}()

	pc := proxyconfig.ProxyConfig{}
	err := pc.InstallInDefaultTransport()
	c.Assert(err, tc.ErrorMatches, `http.DefaultTransport was .*\.fakeRoundTripper instead of \*http\.Transport`)
}

func (s *Suite) TestInstall(c *tc.C) {
	oldTransport := http.DefaultTransport
	defer func() {
		http.DefaultTransport = oldTransport
	}()

	pc := proxyconfig.ProxyConfig{}
	pc.Set(normal)
	pc.InstallInDefaultTransport()

	transport, ok := http.DefaultTransport.(*http.Transport)
	c.Assert(ok, tc.IsTrue)

	req, err := http.NewRequest("GET", "https://risky.biz", nil)
	c.Assert(err, tc.ErrorIsNil)
	proxyURL, err := transport.Proxy(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(proxyURL, tc.Not(tc.IsNil))
	c.Assert(proxyURL.String(), tc.Equals, "https://https.proxy")
}
