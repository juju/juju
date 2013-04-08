package local

import (
	"net"

	"launchpad.net/juju-core/environs/storage"
)

func Listen(basepath, environName, ip string, port int) (net.Listener, error) {
	return listen(basepath, environName, ip, port)
}

func NewStorage(address string, port int) storage.ReadWriter {
	return newStorage(address, port)
}
