// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

// TODO(ericsnow) This file should be removed as soon as possible.
// It is based on https://github.com/lxc/lxd/blob/master/lxc/remote.go
// and the code there should instead be cleaned up and exported so we
// could use it here.

// TODO(ericsnow) Address licensing.

// addServer adds the given remote info to the provided config.
// The implementation is derived from:
//  https://github.com/lxc/lxd/blob/master/lxc/remote.go
// Note that we've removed some validation code (we don't need it).
func addServer(config *lxd.Config, server string, addr string) error {
	addr, err := fixAddr(addr, shared.PathExists)
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

// fixAddr is the portion of the original addServer function that fixes
// up the provided addr to be a fully qualified URL.
func fixAddr(addr string, pathExists func(string) bool) (string, error) {
	if addr == "" {
		// TODO(ericsnow) Return lxd.LocalRemote.Addr?
		return addr, nil
	}

	parsedURL, err := url.Parse(addr)
	if err != nil {
		return "", err
	}
	remoteURL := url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
		//Path:   parsedURL.Path,
	}

	// Fix the scheme.
	switch remoteURL.Scheme {
	case "https":
	case "unix":
	case "":
		// TODO(ericsnow) Decided based on whether or not Host is set?
		remoteURL.Scheme = "https"
		// TODO(ericsnow) Use remoteURL.Path/.Opaque here instead of addr.
		if addr[0] == '/' || pathExists(addr) {
			remoteURL.Scheme = "unix"
		}
	default:
		// TODO(ericsnow) Fail by default?
		remoteURL.Scheme = "https"
	}

	// Fix the host.
	if remoteURL.Host == "" {
		// TODO(ericsnow) Instead, make use of Path and Opaque below.
		remoteURL.Host = addr
	}
	host, port, err := net.SplitHostPort(remoteURL.Host)
	if err != nil {
		port = shared.DefaultPort
	} else {
		remoteURL.Host = host
	}

	// Special-case "unix".
	if remoteURL.Scheme == "unix" {
		// TODO(ericsnow) All this could be done earlier.
		// TODO(ericsnow) Use Path and Opaque instead of addr.
		if addr[0:5] == "unix:" {
			if len(addr) > 6 && addr[0:7] == "unix://" {
				if len(addr) > 8 {
					remoteURL.Host = addr[8:]
				} else {
					remoteURL.Host = ""
				}
			} else {
				remoteURL.Host = addr[6:]
			}
		}
		port = ""
	}

	// Handle IPv6 hosts.
	if strings.Contains(remoteURL.Host, ":") && !strings.HasPrefix(remoteURL.Host, "[") {
		remoteURL.Host = fmt.Sprintf("[%s]", remoteURL.Host)
	}

	// Add the port, if applicable.
	if port != "" {
		remoteURL.Host += ":" + port
	}

	// TODO(ericsnow) Use remoteUrl.String()
	return fmt.Sprintf("%s://%s", remoteURL.Scheme, remoteURL.Host), nil
}
