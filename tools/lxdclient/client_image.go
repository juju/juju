// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/lxc/lxd"

	"github.com/juju/juju/utils/stringforwarder"
)

type rawImageClient interface {
	GetAlias(string) string
}

type remoteClient interface {
	URL() string
	GetAlias(name string) string
	// This is like lxd.Client.CopyImage() but simplified and allows us to
	// inject a testing double.
	CopyImage(imageTarget string, dest rawImageClient, aliases []string, callback func(string)) error
}

type imageClient struct {
	raw             rawImageClient
	connectToSource func(Remote) (remoteClient, error)
}

type rawWrapper struct {
	*lxd.Client
}

func (r rawWrapper) URL() string {
	return r.Client.BaseURL
}

func (r rawWrapper) CopyImage(imageTarget string, dest rawImageClient, aliases []string, callback func(string)) error {
	rawDest, ok := dest.(*lxd.Client)
	if !ok {
		return errors.Errorf("can only copy images to a real lxd.Client instance")
	}
	return r.Client.CopyImage(
		imageTarget,
		rawDest,
		false,   // copy_aliases
		aliases, // create these aliases
		false,   // make the image public
		true,    // autoUpdate,
		callback,
	)
}

func connectToRaw(remote Remote) (remoteClient, error) {
	raw, err := newRawClient(remote)
	if err != nil {
		return nil, err
	}
	return rawWrapper{raw}, nil
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

// EnsureImageExists makes sure we have a local image so we can launch a
// container.
// @param series: OS series (trusty, precise, etc)
// @param architecture: (TODO) The architecture of the image we want to use
// @param trustLocal: (TODO) check if we already have an image with the right alias.
// Setting this to False means we will always check the remote sources and only
// launch the newest version.
// @param sources: a list of Remotes that we will look in for the image.
// @param copyProgressHandler: a callback function. If we have to download an
// image, we will call this with messages indicating how much of the download
// we have completed (and where we are downloading it from).
func (i *imageClient) EnsureImageExists(series string, sources []Remote, copyProgressHandler func(string)) error {
	// TODO(jam) Find a way to test this, even though lxd.Client can't
	// really be stubbed out because CopyImage takes one directly and pokes
	// at private methods so we can't easily tweak it.
	name := i.ImageNameForSeries(series)

	var lastErr error
	for _, remote := range sources {
		source, err := i.connectToSource(remote)
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
			logger.Infof("no image for %s found in %s", name, source.URL())
			// TODO(jam) Add a test that we skip sources that don't
			// have what we are looking for
			continue
		}
		logger.Infof("found image from %s for %s = %s",
			source.URL(), series, target)
		forwarder := stringforwarder.New(copyProgressHandler)
		defer func() {
			dropCount := forwarder.Stop()
			logger.Debugf("dropped %d progress messages", dropCount)
		}()
		adapter := &progressContext{
			logger:  logger,
			level:   loggo.INFO,
			context: fmt.Sprintf("copying image for %s from %s: %%s", name, source.URL()),
			forward: forwarder.Forward,
		}
		err = source.CopyImage(series, i.raw, []string{name}, adapter.copyProgress)
		return errors.Annotatef(err, "unable to get LXD image for %s", name)
	}
	return lastErr
}

// A common place to compute image names (aliases) based on the series
func (i imageClient) ImageNameForSeries(series string) string {
	// TODO(jam) Do we need 'ubuntu' in there? We only need it if "series"
	// would collide, but all our supported series are disjoint
	return fmt.Sprintf("ubuntu-%s", series)
}
