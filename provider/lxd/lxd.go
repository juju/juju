// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/tags"
)

// The metadata keys used when creating new instances.
const (
	metadataKeyIsState = tags.JujuEnv
	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/DataSourceGCE.py
	// http://cloudinit.readthedocs.org/en/latest/
	// https://cloud.google.com/compute/docs/metadata
	metadataKeyCloudInit = "user-data"
	metadataKeyEncoding  = "user-data-encoding"
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
	imageBasePath = "projects/ubuntu-os-cloud/global/images/"
)

var (
	logger = loggo.GetLogger("juju.provider.gce")

	errNotImplemented = errors.NotImplementedf("gce provider functionality")
)
