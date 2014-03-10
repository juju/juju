// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"fmt"
	"os"
	"strings"
)

const (
	// Remove the likelihood of errors by mistyping string values.
	http_proxy  = "http_proxy"
	https_proxy = "https_proxy"
	ftp_proxy   = "ftp_proxy"
	no_proxy    = "no_proxy"
)

// ProxySettings holds the values for the http, https and ftp proxies as well
// as the no_proxy value found by Detect Proxies.
type ProxySettings struct {
	Http    string
	Https   string
	Ftp     string
	NoProxy string
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
		Http:    getProxySetting(http_proxy),
		Https:   getProxySetting(https_proxy),
		Ftp:     getProxySetting(ftp_proxy),
		NoProxy: getProxySetting(no_proxy),
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
	addLine(http_proxy, s.Http)
	addLine(https_proxy, s.Https)
	addLine(ftp_proxy, s.Ftp)
	addLine(no_proxy, s.NoProxy)
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
	addLine(http_proxy, s.Http)
	addLine(https_proxy, s.Https)
	addLine(ftp_proxy, s.Ftp)
	addLine(no_proxy, s.NoProxy)
	return lines
}

// SetEnvironmentValues updates the process environment with the
// proxy values stored in the settings object.  Both the lower-case
// and upper-case variants are set.
//
// http_proxy, HTTP_PROXY
// https_proxy, HTTPS_PROXY
// ftp_proxy, FTP_PROXY
func (s *ProxySettings) SetEnvironmentValues() {
	setenv := func(proxy, value string) {
		os.Setenv(proxy, value)
		os.Setenv(strings.ToUpper(proxy), value)
	}
	setenv(http_proxy, s.Http)
	setenv(https_proxy, s.Https)
	setenv(ftp_proxy, s.Ftp)
	setenv(no_proxy, s.NoProxy)
}
