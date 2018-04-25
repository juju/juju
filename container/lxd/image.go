// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"path"

	"github.com/juju/errors"
	jujuarch "github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

// JujuImageServer extends the upstream LXD client.
type JujuImageServer struct {
	lxd.ImageServer
}

// SourcedImage is the result of a successful image acquisition.
// It includes the relevant data that located the image.
type SourcedImage struct {
	// Image is the actual image data that was located.
	Image *api.Image
	// Alias is the alias that correctly identified the image.
	Alias string
	// LXDServer is the image server that supplied the image.
	LXDServer lxd.ImageServer
	// Remote is our description of the server where the image was found.
	Remote *RemoteServer
}

// FindImage searches the input sources in supplied order, looking for an OS
// image matching the supplied series and architecture.
// If found, the image and the server from which it was acquired are returned.
// If the server is remote the image will be cached by LXD when used to create
// a container.
func (s *JujuImageServer) FindImage(series, arch string, sources []RemoteServer) (SourcedImage, error) {
	lastErr := fmt.Errorf("no matching image found")

	// First we check if we have the image locally.
	localAlias := seriesLocalAlias(series, arch)
	var target string
	entry, _, err := s.ImageServer.GetImageAlias(localAlias)
	if entry != nil {
		// We already have an image with the given alias,
		// so just use that.
		target = entry.Target
		image, _, err := s.ImageServer.GetImage(target)
		if err == nil {
			logger.Debugf("Found image locally - %q %q", image.Filename, target)
			return SourcedImage{
				Image:     image,
				Alias:     localAlias,
				LXDServer: s.ImageServer,
				Remote:    nil,
			}, nil
		}
	}

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of the aliases
	// that might identify the image we want.
	aliases, err := seriesRemoteAliases(series, arch)
	if err != nil {
		return SourcedImage{}, errors.Trace(err)
	}
	for _, remote := range sources {
		source, err := ConnectImageRemote(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = err
			continue
		}
		var foundAlias string
		for _, alias := range aliases {
			if result, _, err := source.GetImageAlias(alias); err == nil && result != nil && result.Target != "" {
				foundAlias = alias
				target = result.Target
				break
			}
		}
		if target != "" {
			image, _, err := source.GetImage(target)
			if err == nil {
				logger.Debugf("Found image remotely - %q %q %q", remote.Name, image.Filename, target)
				return SourcedImage{
					Image:     image,
					Alias:     foundAlias,
					LXDServer: source,
					Remote:    &remote,
				}, nil
			} else {
				lastErr = err
			}
		}
	}
	return SourcedImage{}, lastErr
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
	case jujuos.Ubuntu:
		return []string{path.Join(series, arch)}, nil
	case jujuos.CentOS:
		if series == "centos7" && arch == jujuarch.AMD64 {
			return []string{"centos/7/amd64"}, nil
		}
	case jujuos.OpenSUSE:
		if series == "opensuseleap" && arch == jujuarch.AMD64 {
			return []string{"opensuse/42.2/amd64"}, nil
		}
	}
	return nil, errors.NotSupportedf("series %q", series)
}
