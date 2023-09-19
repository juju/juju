// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"net/http"

	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	proxyconfig "github.com/juju/juju/internal/proxy/config"
)

type Suite struct{}

var _ = gc.Suite(&Suite{})

func checkProxy(c *gc.C, settings proxy.Settings, requestURL, expectedURL string) {
	pc := proxyconfig.ProxyConfig{}
	c.Assert(pc.Set(settings), jc.ErrorIsNil)
	req, err := http.NewRequest("GET", requestURL, nil)
	c.Assert(err, jc.ErrorIsNil)
	proxyURL, err := pc.GetProxy(req)
	c.Assert(err, jc.ErrorIsNil)
	if expectedURL == "" {
		c.Check(proxyURL, gc.IsNil)
	} else {
		c.Assert(proxyURL, gc.Not(gc.IsNil))
		c.Check(proxyURL.String(), gc.Equals, expectedURL)
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

func (s *Suite) TestGetProxy(c *gc.C) {
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

func (s *Suite) TestSetBadUrl(c *gc.C) {
	pc := proxyconfig.ProxyConfig{}
	err := pc.Set(proxy.Settings{
		Https: "http://badurl%gg",
	})
	c.Assert(err, gc.ErrorMatches, `https proxy: invalid proxy address "http://badurl%gg": .*$`)
	err = pc.Set(proxy.Settings{
		Http: "http://badurl%gg",
	})
	c.Assert(err, gc.ErrorMatches, `http proxy: invalid proxy address "http://badurl%gg": .*$`)
}

func (s *Suite) TestInstallError(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, `http.DefaultTransport was \*proxy_test\.fakeRoundTripper instead of \*http\.Transport`)
}

func (s *Suite) TestInstall(c *gc.C) {
	oldTransport := http.DefaultTransport
	defer func() {
		http.DefaultTransport = oldTransport
	}()

	pc := proxyconfig.ProxyConfig{}
	pc.Set(normal)
	pc.InstallInDefaultTransport()

	transport, ok := http.DefaultTransport.(*http.Transport)
	c.Assert(ok, jc.IsTrue)

	req, err := http.NewRequest("GET", "https://risky.biz", nil)
	c.Assert(err, jc.ErrorIsNil)
	proxyURL, err := transport.Proxy(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(proxyURL, gc.Not(gc.IsNil))
	c.Assert(proxyURL.String(), gc.Equals, "https://https.proxy")
}
