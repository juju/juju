// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/tags"
)

// The metadata keys used when creating new instances.
const (
	metadataKeyIsState = tags.JujuModel
	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/DataSourceGCE.py
	// http://cloudinit.readthedocs.org/en/latest/
	// https://cloud.google.com/compute/docs/metadata
	metadataKeyCloudInit       = "user-data"
	metadataKeyEncoding        = "user-data-encoding"
	metadataKeyWindowsUserdata = "windows-startup-script-ps1"
	metadataKeyWindowsSysprep  = "sysprep-specialize-script-ps1"
	// GCE uses this specific key for authentication (*handwaving*)
	// https://cloud.google.com/compute/docs/instances#sshkeys
	metadataKeySSHKeys = "sshKeys"
)

// Common metadata values used when creating new instances.
const (
	metadataValueTrue  = "true"
	metadataValueFalse = "false"
)

const (
	// See https://cloud.google.com/compute/docs/operating-systems/linux-os#ubuntu
	// TODO(ericsnow) Should this be handled in cloud-images (i.e.
	// simplestreams)?
	ubuntuImageBasePath  = "projects/ubuntu-os-cloud/global/images/"
	windowsImageBasePath = "projects/windows-cloud/global/images/"
)

var (
	logger = loggo.GetLogger("juju.provider.gce")

	errNotImplemented = errors.NotImplementedf("gce provider functionality")
)
