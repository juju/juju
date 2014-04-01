// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv_test

import (
	"os"

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
	s.PatchEnvironment("no_proxy", "")
	s.PatchEnvironment("NO_PROXY", "")

	proxies := osenv.DetectProxies()

	c.Assert(proxies, gc.DeepEquals, osenv.ProxySettings{})
}

func (s *proxySuite) TestDetectPrimary(c *gc.C) {
	// Patch all of the environment variables we check out just in case the
	// user has one set.
	s.PatchEnvironment("http_proxy", "http://user@10.0.0.1")
	s.PatchEnvironment("HTTP_PROXY", "")
	s.PatchEnvironment("https_proxy", "https://user@10.0.0.1")
	s.PatchEnvironment("HTTPS_PROXY", "")
	s.PatchEnvironment("ftp_proxy", "ftp://user@10.0.0.1")
	s.PatchEnvironment("FTP_PROXY", "")
	s.PatchEnvironment("no_proxy", "10.0.3.1,localhost")
	s.PatchEnvironment("NO_PROXY", "")

	proxies := osenv.DetectProxies()

	c.Assert(proxies, gc.DeepEquals, osenv.ProxySettings{
		Http:    "http://user@10.0.0.1",
		Https:   "https://user@10.0.0.1",
		Ftp:     "ftp://user@10.0.0.1",
		NoProxy: "10.0.3.1,localhost",
	})
}

func (s *proxySuite) TestDetectFallback(c *gc.C) {
	// Patch all of the environment variables we check out just in case the
	// user has one set.
	s.PatchEnvironment("http_proxy", "")
	s.PatchEnvironment("HTTP_PROXY", "http://user@10.0.0.2")
	s.PatchEnvironment("https_proxy", "")
	s.PatchEnvironment("HTTPS_PROXY", "https://user@10.0.0.2")
	s.PatchEnvironment("ftp_proxy", "")
	s.PatchEnvironment("FTP_PROXY", "ftp://user@10.0.0.2")
	s.PatchEnvironment("no_proxy", "")
	s.PatchEnvironment("NO_PROXY", "10.0.3.1,localhost")

	proxies := osenv.DetectProxies()

	c.Assert(proxies, gc.DeepEquals, osenv.ProxySettings{
		Http:    "http://user@10.0.0.2",
		Https:   "https://user@10.0.0.2",
		Ftp:     "ftp://user@10.0.0.2",
		NoProxy: "10.0.3.1,localhost",
	})
}

func (s *proxySuite) TestDetectPrimaryPreference(c *gc.C) {
	// Patch all of the environment variables we check out just in case the
	// user has one set.
	s.PatchEnvironment("http_proxy", "http://user@10.0.0.1")
	s.PatchEnvironment("https_proxy", "https://user@10.0.0.1")
	s.PatchEnvironment("ftp_proxy", "ftp://user@10.0.0.1")
	s.PatchEnvironment("no_proxy", "10.0.3.1,localhost")
	s.PatchEnvironment("HTTP_PROXY", "http://user@10.0.0.2")
	s.PatchEnvironment("HTTPS_PROXY", "https://user@10.0.0.2")
	s.PatchEnvironment("FTP_PROXY", "ftp://user@10.0.0.2")
	s.PatchEnvironment("NO_PROXY", "localhost")

	proxies := osenv.DetectProxies()

	c.Assert(proxies, gc.DeepEquals, osenv.ProxySettings{
		Http:    "http://user@10.0.0.1",
		Https:   "https://user@10.0.0.1",
		Ftp:     "ftp://user@10.0.0.1",
		NoProxy: "10.0.3.1,localhost",
	})
}

func (s *proxySuite) TestAsScriptEnvironmentEmpty(c *gc.C) {
	proxies := osenv.ProxySettings{}
	c.Assert(proxies.AsScriptEnvironment(), gc.Equals, "")
}

func (s *proxySuite) TestAsScriptEnvironmentOneValue(c *gc.C) {
	proxies := osenv.ProxySettings{
		Http: "some-value",
	}
	expected := `
export http_proxy=some-value
export HTTP_PROXY=some-value`[1:]
	c.Assert(proxies.AsScriptEnvironment(), gc.Equals, expected)
}

func (s *proxySuite) TestAsScriptEnvironmentAllValue(c *gc.C) {
	proxies := osenv.ProxySettings{
		Http:    "some-value",
		Https:   "special",
		Ftp:     "who uses this?",
		NoProxy: "10.0.3.1,localhost",
	}
	expected := `
export http_proxy=some-value
export HTTP_PROXY=some-value
export https_proxy=special
export HTTPS_PROXY=special
export ftp_proxy=who uses this?
export FTP_PROXY=who uses this?
export no_proxy=10.0.3.1,localhost
export NO_PROXY=10.0.3.1,localhost`[1:]
	c.Assert(proxies.AsScriptEnvironment(), gc.Equals, expected)
}

func (s *proxySuite) TestAsEnvironmentValuesEmpty(c *gc.C) {
	proxies := osenv.ProxySettings{}
	c.Assert(proxies.AsEnvironmentValues(), gc.HasLen, 0)
}

func (s *proxySuite) TestAsEnvironmentValuesOneValue(c *gc.C) {
	proxies := osenv.ProxySettings{
		Http: "some-value",
	}
	expected := []string{
		"http_proxy=some-value",
		"HTTP_PROXY=some-value",
	}
	c.Assert(proxies.AsEnvironmentValues(), gc.DeepEquals, expected)
}

func (s *proxySuite) TestAsEnvironmentValuesAllValue(c *gc.C) {
	proxies := osenv.ProxySettings{
		Http:    "some-value",
		Https:   "special",
		Ftp:     "who uses this?",
		NoProxy: "10.0.3.1,localhost",
	}
	expected := []string{
		"http_proxy=some-value",
		"HTTP_PROXY=some-value",
		"https_proxy=special",
		"HTTPS_PROXY=special",
		"ftp_proxy=who uses this?",
		"FTP_PROXY=who uses this?",
		"no_proxy=10.0.3.1,localhost",
		"NO_PROXY=10.0.3.1,localhost",
	}
	c.Assert(proxies.AsEnvironmentValues(), gc.DeepEquals, expected)
}

func (s *proxySuite) TestSetEnvironmentValues(c *gc.C) {
	s.PatchEnvironment("http_proxy", "initial")
	s.PatchEnvironment("HTTP_PROXY", "initial")
	s.PatchEnvironment("https_proxy", "initial")
	s.PatchEnvironment("HTTPS_PROXY", "initial")
	s.PatchEnvironment("ftp_proxy", "initial")
	s.PatchEnvironment("FTP_PROXY", "initial")
	s.PatchEnvironment("no_proxy", "initial")
	s.PatchEnvironment("NO_PROXY", "initial")

	proxy := osenv.ProxySettings{
		Http:  "http proxy",
		Https: "https proxy",
		// Ftp left blank to show clearing env.
		NoProxy: "10.0.3.1,localhost",
	}
	proxy.SetEnvironmentValues()

	obtained := osenv.DetectProxies()

	c.Assert(obtained, gc.DeepEquals, proxy)

	c.Assert(os.Getenv("http_proxy"), gc.Equals, "http proxy")
	c.Assert(os.Getenv("HTTP_PROXY"), gc.Equals, "http proxy")
	c.Assert(os.Getenv("https_proxy"), gc.Equals, "https proxy")
	c.Assert(os.Getenv("HTTPS_PROXY"), gc.Equals, "https proxy")
	c.Assert(os.Getenv("ftp_proxy"), gc.Equals, "")
	c.Assert(os.Getenv("FTP_PROXY"), gc.Equals, "")
	c.Assert(os.Getenv("no_proxy"), gc.Equals, "10.0.3.1,localhost")
	c.Assert(os.Getenv("NO_PROXY"), gc.Equals, "10.0.3.1,localhost")
}
