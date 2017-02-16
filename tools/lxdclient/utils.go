// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"bytes"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/series"
	"github.com/lxc/lxd/shared"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

type closingBuffer struct {
	bytes.Buffer
}

// Close implements io.Closer.
func (closingBuffer) Close() error {
	return nil
}

// IsInstalledLocally returns true if LXD is installed locally.
func IsInstalledLocally() (bool, error) {
	names, err := service.ListServices()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, name := range names {
		if name == "lxd" {
			return true, nil
		}
	}
	return false, nil
}

// IsRunningLocally returns true if LXD is running locally.
func IsRunningLocally() (bool, error) {
	installed, err := IsInstalledLocally()
	if err != nil {
		return installed, errors.Trace(err)
	}
	if !installed {
		return false, nil
	}

	hostSeries, err := series.HostSeries()
	if err != nil {
		return false, errors.Trace(err)
	}
	svc, err := service.NewService("lxd", common.Conf{}, hostSeries)
	if err != nil {
		return false, errors.Trace(err)
	}

	running, err := svc.Running()
	if err != nil {
		return running, errors.Trace(err)
	}

	return running, nil
}

const errIPV6NotSupported = `socket: address family not supported by protocol`

// EnableHTTPSListener configures LXD to listen for HTTPS requests,
// rather than only via the Unix socket.
func EnableHTTPSListener(client interface {
	ServerStatus() (*shared.ServerState, error)
	SetServerConfig(k, v string) error
}) error {
	// First check that the server is not already listening for HTTPS.
	state, err := client.ServerStatus()
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := state.Config["core.https_address"]; ok {
		return nil
	}

	// Make sure the LXD service is configured to listen to local https
	// requests, rather than only via the Unix socket.
	// TODO: jam 2016-02-25 This tells LXD to listen on all addresses,
	//      which does expose the LXD to outside requests. It would
	//      probably be better to only tell LXD to listen for requests on
	//      the loopback and LXC bridges that we are using.
	if err := client.SetServerConfig("core.https_address", "[::]"); err != nil {
		// if the error hints that the problem might be a protocol unsoported
		// such as what happens when IPV6 is disabled in kernel, we try IPV4
		// as a fallback.
		cause := errors.Cause(err)
		if strings.HasSuffix(cause.Error(), errIPV6NotSupported) {
			return errors.Trace(client.SetServerConfig("core.https_address", "0.0.0.0"))
		}
		return errors.Trace(err)
	}
	return nil
}
