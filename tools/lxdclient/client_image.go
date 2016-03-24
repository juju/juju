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

	"github.com/juju/juju/utils/stringforwarder"
)

type rawImageClient interface {
	ListAliases() (shared.ImageAliases, error)
}

type imageClient struct {
	raw rawImageClient
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

func (i *imageClient) EnsureImageExists(series string, sources []Remote, copyProgressHandler func(string)) error {
	// TODO(jam) We should add Architecture in this information as well
	// TODO(jam) We should also update this for multiple locations to copy
	// from
	// TODO(jam) Find a way to test this, even though lxd.Client can't
	// really be stubbed out because CopyImage takes one directly and pokes
	// at private methods so we can't easily tweak it.
	name := i.ImageNameForSeries(series)

	// TODO(jam) Add a flag to not trust local aliases, which would allow
	// non-state machines to only trust the alias that is set on the state
	// machines.
	// if IgnoreLocalAliases {}
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

	client, ok := i.raw.(*lxd.Client)
	if !ok {
		return errors.Errorf("can only copy images to a real lxd.Client instance")
	}
	var lastErr error
	for _, remote := range sources {
		source, err := newRawClient(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = err
			continue
		}

		// TODO(jam): there are multiple possible spellings for aliases,
		// unfortunately. cloud-images only hosts ubuntu images, and
		// aliases them as "trusty" or "trusty/amd64" or
		// "trusty/amd64/20160304". However, we should be more
		// explicit. and use "ubuntu/trusty/amd64" as our default
		// naming scheme, and only fall back for synchronization.
		target := source.GetAlias(series)
		if target == "" {
			logger.Infof("no image for %s found in %s", name, source.BaseURL)
			// TODO(jam) Add a test that we skip sources that don't
			// have what we are looking for
			continue
		}
		logger.Infof("found image from %s for %s = %s",
			source.BaseURL, series, target)
		forwarder := stringforwarder.NewStringForwarder(copyProgressHandler)
		defer func() {
			dropCount := forwarder.Stop()
			logger.Debugf("dropped %d progress messages", dropCount)
		}()
		adapter := &progressContext{
			logger:  logger,
			level:   loggo.INFO,
			context: fmt.Sprintf("copying image for %s from %s: %%s", name, source.BaseURL),
			forward: forwarder.Receive,
		}
		err = source.CopyImage(
			target, client, false, []string{name}, false,
			true, adapter.copyProgress)
		if err != nil {
			// TODO(jam) Should this be fatal? Or just set lastErr
			// and then continue on?
			logger.Warningf("error copying image: %s", err)
			return errors.Annotatef(err, "unable to get LXD image for %s", name)
		}
		return nil
	}
	return lastErr
}

// A common place to compute image names (aliases) based on the series
func (i imageClient) ImageNameForSeries(series string) string {
	return "ubuntu-" + series
}
