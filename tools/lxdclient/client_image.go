// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"os/exec"

	"github.com/lxc/lxd/shared"

	"github.com/juju/errors"
)

type rawImageClient interface {
	ListAliases() (shared.ImageAliases, error)
}

type imageClient struct {
	raw rawImageClient
}

func (i imageClient) EnsureImageExists(series string) error {
	name := i.ImageNameForSeries(series)

	aliases, err := i.raw.ListAliases()
	if err != nil {
		return err
	}

	for _, alias := range aliases {
		if alias.Description == name {
			return nil
		}
	}

	cmd := exec.Command("lxd-images", "import", "ubuntu", series, "--alias", name)
	return errors.Trace(cmd.Run())
}

// A common place to compute image names (alises) based on the series
func (i imageClient) ImageNameForSeries(series string) string {
	return "ubuntu-" + series
}
