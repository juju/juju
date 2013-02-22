package local

import (
	"net"

	"launchpad.net/juju-core/environs"
)

func Listen(basepath, environName, ip string, port int) (net.Listener, error) {
	return listen(basepath, environName, ip, port)
}

func NewStorage(address string, port int) environs.Storage {
	return newStorage(address, port)
}
