// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"bytes"

	"github.com/juju/errors"
	"github.com/juju/utils/series"

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

	series, err := series.HostSeries()
	if err != nil {
		return false, errors.Trace(err)
	}

	svc, err := service.NewService("lxd", common.Conf{}, series)
	if err != nil {
		return false, errors.Trace(err)
	}

	running, err := svc.Running()
	if err != nil {
		return running, errors.Trace(err)
	}

	return running, nil
}

// EnableHTTPSListener configures LXD to listen for HTTPS requests,
// rather than only via the Unix socket.
func EnableHTTPSListener(client interface {
	SetConfig(k, v string) error
}) error {
	// Make sure the LXD service is configured to listen to local https
	// requests, rather than only via the Unix socket.
	// TODO: jam 2016-02-25 This tells LXD to listen on all addresses,
	//      which does expose the LXD to outside requests. It would
	//      probably be better to only tell LXD to listen for requests on
	//      the loopback and LXC bridges that we are using.
	if err := client.SetConfig("core.https_address", "[::]"); err != nil {
		return errors.Trace(err)
	}
	return nil
}
