// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/juju/os/series"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

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
