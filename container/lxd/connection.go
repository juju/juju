// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/lxc/lxd/client"
)

type Protocol string

const (
	LXDProtocol           Protocol = "lxd"
	SimpleStreamsProtocol Protocol = "simplestreams"
)

// RemoteServer describes the location and connection details for a
// server utilised in LXD workflows.
type RemoteServer struct {
	Name     string
	Host     string
	Protocol Protocol
	lxd.ConnectionArgs
}

// CloudImagesRemote hosts releases blessed by the Canonical team.
var CloudImagesRemote = RemoteServer{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/releases",
	Protocol: SimpleStreamsProtocol,
}

// CloudImagesDailyRemote hosts images from daily package builds.
// These images have not been independently tested, but should be sound for
// use, being build from packages in the released archive.
var CloudImagesDailyRemote = RemoteServer{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/daily",
	Protocol: SimpleStreamsProtocol,
}

// ConnectImageRemote connects to a remote ImageServer using specified protocol.
var ConnectImageRemote = connectImageRemote

func connectImageRemote(remote RemoteServer) (lxd.ImageServer, error) {
	switch remote.Protocol {
	case LXDProtocol:
		return lxd.ConnectPublicLXD(remote.Host, &remote.ConnectionArgs)
	case SimpleStreamsProtocol:
		return lxd.ConnectSimpleStreams(remote.Host, &remote.ConnectionArgs)
	}
	return nil, fmt.Errorf("bad protocol supplied for connection: %v", remote.Protocol)
}

// ConnectLocal connects to LXD on a local socket.
func ConnectLocal() (lxd.ContainerServer, error) {
	client, err := lxd.ConnectLXDUnix(SocketPath(), &lxd.ConnectionArgs{})
	return client, errors.Trace(err)
}

var osStat = os.Stat

// SocketPath returns the path to the local LXD socket.
// The following are tried in order of preference:
//   - LXD_DIR environment variable.
//   - Snap socket.
//   - Debian socket.
// We give preference to LXD installed via Snap.
func SocketPath() string {
	path := os.Getenv("LXD_DIR")
	if path != "" {
		logger.Debugf("using environment LXD_DIR as socket path: %q", path)
	} else {
		snapSocket := filepath.FromSlash("/var/snap/lxd/common/lxd")
		if _, err := osStat(snapSocket); err == nil {
			logger.Debugf("using LXD snap socket: %q", snapSocket)
			path = snapSocket
		} else {
			debSocket := filepath.FromSlash("/var/lib/lxd")
			logger.Debugf("LXD snap socket not found, falling back to debian socket: %q", debSocket)
			path = debSocket
		}
	}
	return filepath.Join(path, "unix.socket")
}
