// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"net"

	"launchpad.net/juju-core/environs"
)

var Provider = provider

func Listen(basepath, ip string, port int) (net.Listener, error) {
	return listen(basepath, ip, port)
}

func NewStorage(address string, port int) environs.Storage {
	return newStorage(address, port)
}

func SetDefaultStorageDirs(public, private string) (oldPublic, oldPrivate string) {
	oldPublic, defaultPublicStorageDir = defaultPublicStorageDir, public
	oldPrivate, defaultPrivateStorageDir = defaultPrivateStorageDir, private
	return
}
