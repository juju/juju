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
// It is copied from https://github.com/lxc/lxd/blob/master/lxc/remote.go
// and the code there should instead be exported so we could use it here.

// TODO(ericsnow) Address licensing.

// addServer adds the given remote info to the provided config.
// The implementation is derived from:
//  https://github.com/lxc/lxd/blob/master/lxc/remote.go
// Note that some validation code was removed (we don't need it).
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

// fixAddr is the portion of the original addServer function that fixes
// up the provided addr to be a fully qualified URL.
func fixAddr(addr string) (string, error) {
	var r_scheme string
	var r_host string
	var r_port string

	/* Complex remote URL parsing */
	remote_url, err := url.Parse(addr)
	if err != nil {
		return "", err
	}

	if remote_url.Scheme != "" {
		if remote_url.Scheme != "unix" && remote_url.Scheme != "https" {
			r_scheme = "https"
		} else {
			r_scheme = remote_url.Scheme
		}
	} else if addr[0] == '/' {
		r_scheme = "unix"
	} else {
		if !shared.PathExists(addr) {
			r_scheme = "https"
		} else {
			r_scheme = "unix"
		}
	}

	if remote_url.Host != "" {
		r_host = remote_url.Host
	} else {
		r_host = addr
	}

	host, port, err := net.SplitHostPort(r_host)
	if err == nil {
		r_host = host
		r_port = port
	} else {
		r_port = shared.DefaultPort
	}

	if r_scheme == "unix" {
		if addr[0:5] == "unix:" {
			if addr[0:7] == "unix://" {
				if len(addr) > 8 {
					r_host = addr[8:]
				} else {
					r_host = ""
				}
			} else {
				r_host = addr[6:]
			}
		}
		r_port = ""
	}

	if strings.Contains(r_host, ":") && !strings.HasPrefix(r_host, "[") {
		r_host = fmt.Sprintf("[%s]", r_host)
	}

	if r_port != "" {
		addr = r_scheme + "://" + r_host + ":" + r_port
	} else {
		addr = r_scheme + "://" + r_host
	}

	return addr, nil
}
