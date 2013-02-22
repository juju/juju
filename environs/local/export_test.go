package local

import (
	"net"
)

func Listen(basepath, environName, ip string, port int) (net.Listener, error) {
	return listen(basepath, environName, ip, port)
}
