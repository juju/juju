// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package proxy

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/collections/set"
)

const (
	// Remove the likelihood of errors by mistyping string values.
	http_proxy  = "http_proxy"
	https_proxy = "https_proxy"
	ftp_proxy   = "ftp_proxy"
	no_proxy    = "no_proxy"
)

// Settings holds the values for the HTTP, HTTPS and FTP proxies as well as the
// no_proxy value found by Detect Proxies.
// AutoNoProxy is filled with addresses of controllers, we never want to proxy those
type Settings struct {
	Http        string
	Https       string
	Ftp         string
	NoProxy     string
	AutoNoProxy string
}

func getSetting(key string) string {
	value := os.Getenv(key)
	if value == "" {
		value = os.Getenv(strings.ToUpper(key))
	}
	return value
}

// DetectProxies returns the proxy settings found the environment.
func DetectProxies() Settings {
	return Settings{
		Http:    getSetting(http_proxy),
		Https:   getSetting(https_proxy),
		Ftp:     getSetting(ftp_proxy),
		NoProxy: getSetting(no_proxy),
	}
}

// HasProxySet returns true if there is a proxy value for HTTP, HTTPS or FTP.
func (s *Settings) HasProxySet() bool {
	return s.Http != "" ||
		s.Https != "" ||
		s.Ftp != ""
}

// AsScriptEnvironment returns a potentially multi-line string in a format
// that specifies exported key=value lines. There are two lines for each non-
// empty proxy value, one lower-case and one upper-case.
func (s *Settings) AsScriptEnvironment() string {
	if !s.HasProxySet() {
		return ""
	}
	var lines []string
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
	addLine(no_proxy, s.FullNoProxy())
	return strings.Join(lines, "\n")
}

// AsEnvironmentValues returns a slice of strings of the format "key=value"
// suitable to be used in a command environment. There are two values for each
// non-empty proxy value, one lower-case and one upper-case.
func (s *Settings) AsEnvironmentValues() []string {
	if !s.HasProxySet() {
		return nil
	}
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
	addLine(no_proxy, s.FullNoProxy())
	return lines
}

// AsSystemdEnvSettings returns a string in the format understood by systemd:
// DefaultEnvironment="http_proxy=...." "HTTP_PROXY=..." ...
func (s *Settings) AsSystemdDefaultEnv() string {
	lines := s.AsEnvironmentValues()
	rv := `# To allow juju to control the global systemd proxy settings,
# create symbolic links to this file from within /etc/systemd/system.conf.d/
# and /etc/systemd/users.conf.d/.
[Manager]
DefaultEnvironment=`
	for _, line := range lines {
		rv += fmt.Sprintf(`"%s" `, line)
	}
	return rv + "\n"
}

// SetEnvironmentValues updates the process environment with the
// proxy values stored in the settings object.  Both the lower-case
// and upper-case variants are set.
//
// http_proxy, HTTP_PROXY
// https_proxy, HTTPS_PROXY
// ftp_proxy, FTP_PROXY
func (s *Settings) SetEnvironmentValues() {
	setenv := func(proxy, value string) {
		os.Setenv(proxy, value)
		os.Setenv(strings.ToUpper(proxy), value)
	}
	setenv(http_proxy, s.Http)
	setenv(https_proxy, s.Https)
	setenv(ftp_proxy, s.Ftp)
	setenv(no_proxy, s.FullNoProxy())
}

// FullNoProxy merges NoProxy and AutoNoProxyList
func (s *Settings) FullNoProxy() string {
	var allNoProxy []string
	if s.NoProxy != "" {
		allNoProxy = strings.Split(s.NoProxy, ",")
	}
	if s.AutoNoProxy != "" {
		allNoProxy = append(allNoProxy, strings.Split(s.AutoNoProxy, ",")...)
	}
	noProxySet := set.NewStrings(allNoProxy...)
	return strings.Join(noProxySet.SortedValues(), ",")
}
