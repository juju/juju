// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing/testbase"
)

type proxySuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&proxySuite{})

func (s *proxySuite) TestDetectNoSettings(c *gc.C) {
	// Patch all of the environment variables we check out just in case the
	// user has one set.
	s.PatchEnvironment("http_proxy", "")
	s.PatchEnvironment("HTTP_PROXY", "")
	s.PatchEnvironment("https_proxy", "")
	s.PatchEnvironment("HTTPS_PROXY", "")
	s.PatchEnvironment("ftp_proxy", "")
	s.PatchEnvironment("FTP_PROXY", "")

	proxies := osenv.DetectProxies()

	c.Assert(proxies.Http, gc.Equals, "")
	c.Assert(proxies.Https, gc.Equals, "")
	c.Assert(proxies.Ftp, gc.Equals, "")
}

func (s *proxySuite) TestDetectFallback(c *gc.C) {
	// Patch all of the environment variables we check out just in case the
	// user has one set.
	s.PatchEnvironment("http_proxy", "http://user@10.0.0.1")
	s.PatchEnvironment("HTTP_PROXY", "")
	s.PatchEnvironment("https_proxy", "")
	s.PatchEnvironment("HTTPS_PROXY", "https://user@10.0.0.2")
	s.PatchEnvironment("ftp_proxy", "")
	s.PatchEnvironment("FTP_PROXY", "")

	proxies := osenv.DetectProxies()

	c.Assert(proxies.Http, gc.Equals, "http://user@10.0.0.1")
	c.Assert(proxies.Https, gc.Equals, "https://user@10.0.0.2")
	c.Assert(proxies.Ftp, gc.Equals, "")
}
