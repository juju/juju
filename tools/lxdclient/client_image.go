// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

type rawImageClient interface {
	ListAliases() (shared.ImageAliases, error)
}

type imageClient struct {
	raw    rawImageClient
	config Config
}

// progressContext takes progress messages from LXD and just writes them to
// the associated logger at the given log level.
type progressContext struct {
	logger  loggo.Logger
	level   loggo.Level
	context string       // a format string that should take a single %s parameter
	forward func(string) // pass messages onward
}

func (p *progressContext) copyProgress(progress string) {
	msg := fmt.Sprintf(p.context, progress)
	p.logger.Logf(p.level, msg)
	if p.forward != nil {
		p.forward(msg)
	}
}

func (i *imageClient) EnsureImageExists(series string, copyProgressHandler func(string)) error {
	// TODO(jam) We should add Architecture in this information as well
	// TODO(jam) We should also update this for multiple locations to copy
	// from
	name := i.ImageNameForSeries(series)

	aliases, err := i.raw.ListAliases()
	if err != nil {
		return err
	}

	for _, alias := range aliases {
		if alias.Description == name {
			logger.Infof("found cached image %q = %s",
				alias.Description, alias.Target)
			return nil
		}
	}

	ubuntu, err := lxdClientForCloudImages(i.config)
	if err != nil {
		return err
	}

	client, ok := i.raw.(*lxd.Client)
	if !ok {
		return errors.Errorf("can't use a fake client as target")
	}
	adapter := &progressContext{
		logger:  logger,
		level:   loggo.INFO,
		context: fmt.Sprintf("copying image for %s from %s: %%s", name, ubuntu.BaseURL),
		forward: copyProgressHandler,
	}
	target := ubuntu.GetAlias(series)
	logger.Infof("found image from %s for %s = %s",
		ubuntu.BaseURL, series, target)
	return ubuntu.CopyImage(
		target, client, false, []string{name}, false,
		true, adapter.copyProgress)
}

// A common place to compute image names (alises) based on the series
func (i imageClient) ImageNameForSeries(series string) string {
	return "ubuntu-" + series
}
