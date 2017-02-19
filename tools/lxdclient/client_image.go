// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/utils/stringforwarder"
)

type rawImageClient interface {
	GetAlias(string) string
	GetImageInfo(string) (*api.Image, error)
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
// @param architecture: The architecture of the image we want to use
// @param trustLocal: (TODO) check if we already have an image with the right alias.
// Setting this to False means we will always check the remote sources and only
// launch the newest version.
// @param sources: a list of Remotes that we will look in for the image.
// @param copyProgressHandler: a callback function. If we have to download an
// image, we will call this with messages indicating how much of the download
// we have completed (and where we are downloading it from).
func (i *imageClient) EnsureImageExists(
	series, arch string,
	sources []Remote,
	copyProgressHandler func(string),
) (string, error) {
	// TODO(jam) Find a way to test this, even though lxd.Client can't
	// really be stubbed out because CopyImage takes one directly and pokes
	// at private methods so we can't easily tweak it.

	// First check if the image exists locally.
	//
	// NOTE(axw) if/when we cache images at the controller, we should
	// revisit the policy around locally cached images. The images will
	// auto-update *eventually*, but we may not want to allow them to
	// be out-of-sync for an extended period of time.
	var lastErr error
	imageName := seriesLocalAlias(series, arch)
	target := i.raw.GetAlias(imageName)
	if target != "" {
		// We already have an image with the given alias,
		// so just use that.
		return imageName, nil
	}

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of
	// the expected aliases.
	aliases, err := seriesRemoteAliases(series, arch)
	if err != nil {
		return "", errors.Trace(err)
	}
	for _, remote := range sources {
		source, err := i.connectToSource(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = err
			continue
		}
		err = i.ensureImage(series, imageName, aliases, source, copyProgressHandler)
		if errors.IsNotFound(err) {
			continue
		}
		if lastErr = err; lastErr == nil {
			break
		}
	}
	return imageName, lastErr
}

func (i *imageClient) ensureImage(
	series, imageName string,
	aliases []string,
	source remoteClient,
	copyProgressHandler func(string),
) error {
	// Look for an image with any of the aliases.
	var alias, target string
	for _, alias = range aliases {
		if target = source.GetAlias(alias); target != "" {
			break
		}
	}
	if target == "" {
		// TODO(jam) Add a test that we skip sources that don't
		// have what we are looking for
		logger.Infof("no image for %s found in %s", imageName, source.URL())
		return errors.NotFoundf("image for %s in %s", imageName, source.URL())
	}

	logger.Infof("found image from %s for %s = %s", source.URL(), imageName, target)
	forwarder := stringforwarder.New(copyProgressHandler)
	defer func() {
		dropCount := forwarder.Stop()
		logger.Debugf("dropped %d progress messages", dropCount)
	}()
	adapter := &progressContext{
		logger:  logger,
		level:   loggo.INFO,
		context: fmt.Sprintf("copying image for %s from %s: %%s", imageName, source.URL()),
		forward: forwarder.Forward,
	}
	err := source.CopyImage(alias, i.raw, []string{imageName}, adapter.copyProgress)
	return errors.Annotatef(err, "unable to get LXD image for %s", imageName)
}

// seriesLocalAlias returns the alias to assign to images for the
// specified series. The alias is juju-specific, to support the
// user supplying a customised image (e.g. CentOS with cloud-init).
func seriesLocalAlias(series, arch string) string {
	return fmt.Sprintf("juju/%s/%s", series, arch)
}

// seriesRemoteAliases returns the aliases to look for in remotes.
func seriesRemoteAliases(series, arch string) ([]string, error) {
	seriesOS, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch seriesOS {
	case os.Ubuntu:
		return []string{path.Join(series, arch)}, nil
	case os.CentOS:
		if series == "centos7" && arch == jujuarch.AMD64 {
			return []string{"centos/7/amd64"}, nil
		}
	}
	return nil, errors.NotSupportedf("series %q", series)
}
