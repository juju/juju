// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"bytes"
	"os"

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

	svc, err := service.NewService("lxd", common.Conf{}, series.HostSeries())
	if err != nil {
		return false, errors.Trace(err)
	}

	running, err := svc.Running()
	if err != nil {
		return running, errors.Trace(err)
	}

	return running, nil
}

// GetDefaultBridgeName returns the name of the default bridge for lxd.
func GetDefaultBridgeName() (string, error) {
	_, err := os.Lstat("/sys/class/net/lxdbr0/bridge")
	if err == nil {
		return "lxdbr0", nil
	}

	/* if it was some unknown error, return that */
	if !os.IsNotExist(err) {
		return "", err
	}

	return "lxcbr0", nil
}
