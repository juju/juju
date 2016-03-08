// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/lxc/lxd"
)

type imageClient struct {
	raw *lxd.Client
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

	/* "ubuntu" here is cloud-images.ubuntu.com's "releases" stream;
	 * "ubuntu-daily" would be the daily stream
	 */
	ubuntu, err := lxd.NewClient(&lxd.DefaultConfig, "ubuntu")
	if err != nil {
		return err
	}

	return ubuntu.CopyImage(series, i.raw, false, []string{name}, false, true, nil)
}

// A common place to compute image names (alises) based on the series
func (i imageClient) ImageNameForSeries(series string) string {
	return "ubuntu-" + series
}
