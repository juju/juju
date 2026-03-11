// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/juju/errors"
	proxyutils "github.com/juju/proxy"
)

// ProxyConfig stores the proxy settings that should be used for web
// requests made from this process.
type ProxyConfig struct {
	mu          sync.Mutex
	http, https *url.URL
	noProxy     string
}

// Set updates the stored settings to the new ones passed in.
func (pc *ProxyConfig) Set(newSettings proxyutils.Settings) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	httpUrl, err := tolerantParse(newSettings.Http)
	if err != nil {
		return errors.Annotate(err, "http proxy")
	}
	httpsUrl, err := tolerantParse(newSettings.Https)
	if err != nil {
		return errors.Annotate(err, "https proxy")
	}
	pc.http = httpUrl
	pc.https = httpsUrl
	pc.noProxy = newSettings.FullNoProxy()
	return nil
}

// GetProxy returns the URL of the proxy to use for a given request as
// indicated by the proxy settings. It behaves the same as the
// net/http.ProxyFromEnvironment function, except that it uses the
// stored settings rather than pulling the configuration from
// environment variables. (The implementation is copied from
// net/http.ProxyFromEnvironment.)
func (pc *ProxyConfig) GetProxy(req *http.Request) (*url.URL, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	var proxy *url.URL
	if req.URL.Scheme == "https" {
		proxy = pc.https
	}
	if proxy == nil {
		proxy = pc.http
	}
	if proxy == nil {
		return nil, nil
	}
	if !pc.useProxy(canonicalAddr(req.URL)) {
		return nil, nil
	}
	return proxy, nil
}

// useProxy reports whether requests to addr should use a proxy,
// according to the NoProxy value of the proxy setting.
// addr is always a canonicalAddr with a host and port.
// (Implementation copied from net/http.useProxy.)
func (pc *ProxyConfig) useProxy(addr string) bool {
	if len(addr) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return false
	}

	if pc.noProxy == "*" {
		return false
	}

	addr = strings.ToLower(strings.TrimSpace(addr))
	if hasPort(addr) {
		addr = addr[:strings.LastIndex(addr, ":")]
	}

	for _, p := range strings.Split(pc.noProxy, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if len(p) == 0 {
			continue
		}
		if hasPort(p) {
			p = p[:strings.LastIndex(p, ":")]
		}
		if addr == p {
			return false
		}
		if p[0] == '.' && (strings.HasSuffix(addr, p) || addr == p[1:]) {
			// no_proxy ".foo.com" matches "bar.foo.com" or "foo.com"
			return false
		}
		if p[0] != '.' && strings.HasSuffix(addr, p) && addr[len(addr)-len(p)-1] == '.' {
			// no_proxy "foo.com" matches "bar.foo.com"
			return false
		}
		if _, net, err := net.ParseCIDR(p); ip != nil && err == nil && net.Contains(ip) {
			return false
		}
	}
	return true
}

// InstallInDefaultTransport sets the proxy resolution used by the
// default HTTP transport to use the proxy details stored in this
// ProxyConfig. Requests made without an explicit transport will
// respect these proxy settings.
func (pc *ProxyConfig) InstallInDefaultTransport() error {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return errors.Errorf("http.DefaultTransport was %T instead of *http.Transport", http.DefaultTransport)
	}
	transport.Proxy = pc.GetProxy
	return nil
}

var DefaultConfig = ProxyConfig{}

func tolerantParse(value string) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}
	proxyURL, err := url.Parse(value)
	if err != nil || !strings.HasPrefix(proxyURL.Scheme, "http") {
		// proxy was bogus. Try prepending "http://" to it and
		// see if that parses correctly. If not, we fall
		// through and complain about the original one.
		if proxyURL, err := url.Parse("http://" + value); err == nil {
			return proxyURL, nil
		}
	}
	if err != nil {
		return nil, errors.Errorf("invalid proxy address %q: %v", value, err)
	}
	return proxyURL, nil
}

// Internal utilities copied from net/http/transport.go
var portMap = map[string]string{
	"http":  "80",
	"https": "443",
}

// canonicalAddr returns url.Host but always with a ":port" suffix
func canonicalAddr(url *url.URL) string {
	addr := url.Host
	if !hasPort(addr) {
		if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
			addr = addr[1 : len(addr)-1]
		}
		return net.JoinHostPort(addr, portMap[url.Scheme])
	}
	return addr
}

// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }
