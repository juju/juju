// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/loggo"
)

// The metadata keys used when creating new instances.
const (
	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/DataSourceGCE.py
	// http://cloudinit.readthedocs.org/en/latest/
	// https://cloud.google.com/compute/docs/metadata
	metadataKeyCloudInit       = "user-data"
	metadataKeyEncoding        = "user-data-encoding"
	metadataKeyWindowsUserdata = "windows-startup-script-ps1"
	metadataKeyWindowsSysprep  = "sysprep-specialize-script-ps1"
)

const (
	// See https://cloud.google.com/compute/docs/operating-systems/linux-os#ubuntu
	// TODO(ericsnow) Should this be handled in cloud-images (i.e.
	// simplestreams)?
	ubuntuImageBasePath      = "projects/ubuntu-os-cloud/global/images/"
	ubuntuDailyImageBasePath = "projects/ubuntu-os-cloud-devel/global/images/"
	windowsImageBasePath     = "projects/windows-cloud/global/images/"
)

var (
	logger = loggo.GetLogger("juju.provider.gce")
)
