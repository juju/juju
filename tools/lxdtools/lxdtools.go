// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdtools

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

var (
	logger                  = loggo.GetLogger("juju.tools.lxdtools")
	lxdConnectPublicLXD     = lxd.ConnectPublicLXD
	lxdConnectSimpleStreams = lxd.ConnectSimpleStreams
	osStat                  = os.Stat
)

type Protocol string

const (
	LXDProtocol           Protocol = "lxd"
	SimplestreamsProtocol Protocol = "simplestreams"
	UserDataKey                    = "user.user-data"
	NetworkConfigKey               = "user.network-config"
	JujuModelKey                   = "user.juju-model"
	AutoStartKey                   = "boot.autostart"
)

type RemoteServer struct {
	Name     string
	Host     string
	Protocol Protocol
	lxd.ConnectionArgs
}

// connectToSource connects to remote ImageServer using specified protocol.
func connectToSource(remote RemoteServer) (lxd.ImageServer, error) {
	switch remote.Protocol {
	case LXDProtocol:
		return lxdConnectPublicLXD(remote.Host, &remote.ConnectionArgs)
	case SimplestreamsProtocol:
		return lxdConnectSimpleStreams(remote.Host, &remote.ConnectionArgs)
	}
	return nil, fmt.Errorf("Wrong protocol %s", remote.Protocol)
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

// GetImageWithServer returns an ImageServer and Image that has the image
// for series and architecture that we're looking for. If the server
// is remote the image will be cached by LXD, we don't need to cache
// it.
func GetImageWithServer(
	server lxd.ImageServer,
	series, arch string,
	sources []RemoteServer) (lxd.ImageServer, *api.Image, string, error) {
	// First we check if we have the image locally.
	lastErr := fmt.Errorf("Image not found")
	imageName := seriesLocalAlias(series, arch)
	var target string
	entry, _, err := server.GetImageAlias(imageName)
	if entry != nil {
		// We already have an image with the given alias,
		// so just use that.
		target = entry.Target
		image, _, err := server.GetImage(target)
		if err == nil {
			logger.Debugf("Found image locally - %q %q", image, target)
			return server, image, target, nil
		}
	}

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of
	// the expected aliases. We don't need to copy this image as
	// it will be cached by LXD.
	aliases, err := seriesRemoteAliases(series, arch)
	if err != nil {
		return nil, nil, "", errors.Trace(err)
	}
	for _, remote := range sources {
		source, err := connectToSource(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = err
			continue
		}
		for _, alias := range aliases {
			if result, _, err := source.GetImageAlias(alias); err == nil && result.Target != "" {
				target = result.Target
				break
			}
		}
		if target != "" {
			image, _, err := source.GetImage(target)
			if err == nil {
				logger.Debugf("Found image remotely - %q %q %q", source, image, target)
				return source, image, target, nil
			} else {
				lastErr = err
			}
		}
	}
	return nil, nil, "", lastErr
}

// LxdSocketPath returns path to local LXD socket.
// First choice is LXD_DIR env variable, second snap socket, third deb socket.
func LxdSocketPath() string {
	// LXD socket is different depending on installation method
	// We prefer upstream's preference of snap installed LXD
	debianSocket := filepath.FromSlash("/var/lib/lxd")
	snapSocket := filepath.FromSlash("/var/snap/lxd/common/lxd")
	path := os.Getenv("LXD_DIR")
	if path != "" {
		logger.Debugf("Using environment LXD_DIR as socket path: %q", path)
	} else {
		if _, err := osStat(snapSocket); err == nil {
			logger.Debugf("Using LXD snap socket: %q", snapSocket)
			path = snapSocket
		} else {
			logger.Debugf("LXD snap socket not found, falling back to debian socket: %q", debianSocket)
			path = debianSocket
		}
	}
	return filepath.Join(path, "unix.socket")
}
