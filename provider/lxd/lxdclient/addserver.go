// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

// addServer adds the given remote info to the provided config.
// The implementation is based loosely on:
//  https://github.com/lxc/lxd/blob/master/lxc/remote.go
func addServer(config *lxd.Config, server string, addr string) error {
	addr, err := fixAddr(addr)
	if err != nil {
		return err
	}

	if config.Remotes == nil {
		config.Remotes = make(map[string]lxd.RemoteConfig)
	}

	/* Actually add the remote */
	// TODO(ericsnow) Fail on collision?
	config.Remotes[server] = lxd.RemoteConfig{Addr: addr}

	return nil
}

// TODO(ericsnow) Rename addr -> remoteURL?

func fixAddr(addr string) (string, error) {
	if addr == "" {
		// TODO(ericsnow) Return lxd.LocalRemote.Addr?
		return addr, nil
	}
	if strings.HasPrefix(addr, "unix:") {
		return "", errors.NewNotValid(nil, fmt.Sprintf("unix socket URLs not supported (got %q)", addr))
	}

	// Fix IPv6 URLs.
	if strings.HasPrefix(addr, ":") {
		parts := strings.SplitN(addr, "/", 2)
		if net.ParseIP(parts[0]) != nil {
			addr = fmt.Sprintf("[%s]", parts[0])
			if len(parts) == 2 {
				addr = "/" + parts[1]
			}
		}
	}

	parsedURL, err := url.Parse(addr)
	if err != nil {
		return "", errors.Trace(err)
	}
	if parsedURL.RawQuery != "" {
		return "", errors.NewNotValid(nil, fmt.Sprintf("URL queries not supported (got %q)", addr))
	}
	if parsedURL.Fragment != "" {
		return "", errors.NewNotValid(nil, fmt.Sprintf("URL fragments not supported (got %q)", addr))
	}
	if parsedURL.Opaque != "" {
		if strings.Contains(parsedURL.Scheme, ".") {
			addr, err := fixAddr("https://" + addr)
			if err != nil {
				return "", errors.Trace(err)
			}
			return addr, nil
		}
		return "", errors.NewNotValid(nil, fmt.Sprintf("opaque URLs not supported (got %q)", addr))
	}

	remoteURL := url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
		Path:   strings.TrimRight(parsedURL.Path, "/"),
	}

	// Fix the scheme.
	remoteURL.Scheme = fixScheme(remoteURL)
	if err := validateScheme(remoteURL); err != nil {
		return "", errors.Trace(err)
	}

	// Fix the host.
	if remoteURL.Host == "" {
		if strings.HasPrefix(remoteURL.Path, "/") {
			return "", errors.NewNotValid(nil, fmt.Sprintf("unix socket URLs not supported (got %q)", addr))
		}
		addr = fmt.Sprintf("%s://%s%s", remoteURL.Scheme, remoteURL.Host, remoteURL.Path)
		addr, err := fixAddr(addr)
		if err != nil {
			return "", errors.Trace(err)
		}
		return addr, nil
	}
	remoteURL.Host = fixHost(remoteURL.Host, shared.DefaultPort)
	if err := validateHost(remoteURL); err != nil {
		return "", errors.Trace(err)
	}

	// TODO(ericsnow) Use remoteUrl.String()
	return fmt.Sprintf("%s://%s%s", remoteURL.Scheme, remoteURL.Host, remoteURL.Path), nil
}

func fixScheme(url url.URL) string {
	switch url.Scheme {
	case "https":
		return url.Scheme
	case "http":
		return "https"
	case "":
		return "https"
	default:
		return url.Scheme
	}
}

func validateScheme(url url.URL) error {
	switch url.Scheme {
	case "https":
	default:
		return errors.NewNotValid(nil, fmt.Sprintf("unsupported URL scheme %q", url.Scheme))
	}
	return nil
}

func fixHost(host, defaultPort string) string {
	// Handle IPv6 hosts.
	if strings.Count(host, ":") > 1 {
		if !strings.HasPrefix(host, "[") {
			return fmt.Sprintf("[%s]:%s", host, defaultPort)
		} else if !strings.Contains(host, "]:") {
			return host + ":" + defaultPort
		}
		return host
	}

	// Handle ports.
	if !strings.Contains(host, ":") {
		return host + ":" + defaultPort
	}

	return host
}

func validateHost(url url.URL) error {
	if url.Host == "" {
		return errors.NewNotValid(nil, "URL missing host")
	}

	host, port, err := net.SplitHostPort(url.Host)
	if err != nil {
		return errors.NewNotValid(err, "")
	}

	// Check the host.
	if net.ParseIP(host) == nil {
		if err := validateDomainName(host); err != nil {
			return errors.Trace(err)
		}
	}

	// Check the port.
	if p, err := strconv.Atoi(port); err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("invalid port in host %q", url.Host))
	} else if p <= 0 || p > 0xFFFF {
		return errors.NewNotValid(err, fmt.Sprintf("invalid port in host %q", url.Host))
	}

	return nil
}

func validateDomainName(fqdn string) error {
	// TODO(ericsnow) Do checks for a valid domain name.

	return nil
}
