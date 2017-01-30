// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/tools/lxdclient"
)

// The metadata keys used when creating new instances.
const (
	metadataKeyCloudInit = lxdclient.UserdataKey
)

var (
	logger = loggo.GetLogger("juju.provider.lxd")
)
