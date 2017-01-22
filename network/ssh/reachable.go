// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"github.com/juju/errors"
	"github.com/juju/juju/network"

	"github.com/juju/loggo"
)


var logger = loggo.GetLogger("juju.network.ssh")

// Dialer defines a Dial() method matching the signature of net.Dial().
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// ReachableHostPort dials the entries in the given hostPorts, in parallel,
// using the given dialer, closing successfully established connections
// after checking the ssh key. Individual connection errors are discarded, and
// an error is returned only if none of the hostPorts can be reached when the
// given timeout expires.
// If publicKeys is a non empty list, then the SSH host public key will be
// checked. If it is not in the list, then that host is not considered valid.
//
// Usually, a net.Dialer initialized with a non-empty Timeout field is passed
// for dialer.
func ReachableHostPort(hostPorts []network.HostPort, publicKeys []string, dialer Dialer, timeout time.Duration) (network.HostPort, error) {
	uniqueHPs := network.UniqueHostPorts(hostPorts)
	successful := make(chan network.HostPort, 1)
	done := make(chan struct{}, 0)

	for _, hostPort := range uniqueHPs {
		go func(hp network.HostPort) {
			conn, err := dialer.Dial("tcp", hp.NetAddr())
			if err == nil {
				conn.Close()
				select {
				case successful <- hp:
					return
				case <-done:
					return
				}
			}
		}(hostPort)
	}

	select {
	case result := <-successful:
		logger.Infof("dialed %q successfully", result)
		close(done)
		return result, nil

	case <-time.After(timeout):
		close(done)
		return network.HostPort{}, errors.Errorf("cannot connect to any address: %v", hostPorts)
	}
}
