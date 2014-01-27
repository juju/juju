// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"fmt"
	"os"
	"strings"
)

// ProxySettings holds the values for the http, https and ftp proxies found by
// Detect Proxies.
type ProxySettings struct {
	Http  string
	Https string
	Ftp   string
}

func getProxySetting(key string) string {
	value := os.Getenv(key)
	if value == "" {
		value = os.Getenv(strings.ToUpper(key))
	}
	return value
}

// DetectProxies returns the proxy settings found the environment.
func DetectProxies() ProxySettings {
	return ProxySettings{
		Http:  getProxySetting("http_proxy"),
		Https: getProxySetting("https_proxy"),
		Ftp:   getProxySetting("ftp_proxy"),
	}
}

// AsScriptEnvironment returns a potentially multi-line string in a format
// that specifies exported key=value lines. There are two lines for each non-
// empty proxy value, one lower-case and one upper-case.
func (s *ProxySettings) AsScriptEnvironment() string {
	lines := []string{}
	addLine := func(proxy, value string) {
		if value != "" {
			lines = append(
				lines,
				fmt.Sprintf("export %s=%s", proxy, value),
				fmt.Sprintf("export %s=%s", strings.ToUpper(proxy), value))
		}
	}
	addLine("http_proxy", s.Http)
	addLine("https_proxy", s.Https)
	addLine("ftp_proxy", s.Ftp)
	return strings.Join(lines, "\n")
}

// AsEnvironmentValues returns a slice of strings of the format "key=value"
// suitable to be used in a command environment. There are two values for each
// non-empty proxy value, one lower-case and one upper-case.
func (s *ProxySettings) AsEnvironmentValues() []string {
	lines := []string{}
	addLine := func(proxy, value string) {
		if value != "" {
			lines = append(
				lines,
				fmt.Sprintf("%s=%s", proxy, value),
				fmt.Sprintf("%s=%s", strings.ToUpper(proxy), value))
		}
	}
	addLine("http_proxy", s.Http)
	addLine("https_proxy", s.Https)
	addLine("ftp_proxy", s.Ftp)
	return lines
}

// SetEnvironmentValues updates the process environment with the
// proxy values stored in the settings object.  Both the lower-case
// and upper-case variants are set.
//
// http-proxy, HTTP_PROXY
// https-proxy, HTTPS_PROXY
// ftp-proxy, FTP_PROXY
func (s *ProxySettings) SetEnvironmentValues() {
	os.Setenv("http-proxy", s.Http)
	os.Setenv("HTTP-PROXY", s.Http)
	os.Setenv("https-proxy", s.Https)
	os.Setenv("HTTPS-PROXY", s.Https)
	os.Setenv("ftp-proxy", s.Ftp)
	os.Setenv("FTP-PROXY", s.Ftp)
}
