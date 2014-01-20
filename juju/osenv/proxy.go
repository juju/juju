// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
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
