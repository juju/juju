// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"strings"

	"gopkg.in/goose.v2/nova"
)

func acceptRackspaceFlavor(d nova.FlavorDetail) bool {
	// On Rackspace, the "compute" and "memory" class
	// flavors do not have ephemeral root disks. You
	// can only boot them with a Cinder volume.
	//
	// TODO(axw) 2016-11-18 #1642795
	// Support flavors without a root disk by
	// creating a bootable Cinder volume.
	if strings.HasPrefix(d.Id, "compute") || strings.HasPrefix(d.Id, "memory") {
		return false
	}
	return true
}
