// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/container/lxd/lxd_client"
	"github.com/juju/juju/environs/tags"
)

// The metadata keys used when creating new instances.
const (
	metadataKeyIsState   = tags.JujuEnv
	metadataKeyCloudInit = lxd_client.UserdataKey
)

// Common metadata values used when creating new instances.
const (
	metadataValueTrue  = "true"
	metadataValueFalse = "false"
)

var (
	logger = loggo.GetLogger("juju.provider.lxd")
)
