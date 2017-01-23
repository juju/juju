// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.network.ssh")

// Dialer defines a Dial() method matching the signature of net.Dial().
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// hostKeyChecker checks if this host matches one of allowed public keys
// it uses the golang/x/crypto/ssh/HostKeyCallback to find the host keys on a
// given connection.
type hostKeyChecker struct {
	// acceptedKeys is a set of the Marshalled PublicKey content.
	acceptedKeys set.Strings
	// stop will be polled for whether we should stop trying to do any work
	stop <-chan struct{}
	// hostPort is the identifier that corresponds to this connection
	hostPort network.HostPort
	// accepted will be passed hostPort if it validated the connection
	accepted chan network.HostPort
}

var hostKeyNotInList = errors.New("host key not in expected set")

func (h *hostKeyChecker) hostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	// Note: we don't do any advanced checking of the PublicKey, like whether
	// the key is revoked or expired. All we care about is whether it matches
	// the public keys that we consider acceptable
	logger.Debugf("checking host key for %q at %v, with key %q", hostname, remote, ssh.MarshalAuthorizedKey(key))

	lookupKey := string(key.Marshal())
	if len(h.acceptedKeys) == 0 || h.acceptedKeys.Contains(lookupKey) {
		logger.Debugf("accepted host key for: %q %v", hostname, remote)
		// This key was valid, so return it, but if someone else was found
		// first, still exit.
		select {
		case h.accepted <- h.hostPort:
		case <-h.stop:
		}
		return nil
	}
	logger.Debugf("host key for %q %v not in our accepted set", hostname, remote)
	return hostKeyNotInList
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
	stop := make(chan struct{}, 0)
	// We use a channel instead of a sync.WaitGroup so that we can return as
	// soon as we get one connected. We'll signal the rest to stop via the
	// 'stop' channel.
	finished := make(chan struct{}, len(uniqueHPs))

	acceptedKeys := set.NewStrings()
	for _, pubKey := range publicKeys {
		// key, comment, options, rest, err
		sshKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			logger.Warningf("unable to handle public key: %q\n", pubKey)
			continue
		}
		acceptedKeys.Add(string(sshKey.Marshal()))
	}
	for _, hostPort := range uniqueHPs {
		go func(hostPort network.HostPort) {
			defer func() {
				select {
				case finished <- struct{}{}:
				case <-stop:
				}
			}()
			checker := &hostKeyChecker{
				acceptedKeys: acceptedKeys,
				stop:         stop,
				accepted:     successful,
				hostPort:     hostPort,
			}
			sshconfig := &ssh.ClientConfig{
				HostKeyCallback: checker.hostKeyCallback,
			}
			addr := hostPort.NetAddr()
			logger.Debugf("dialing %q", addr)
			conn, err := dialer.Dial("tcp", addr)
			if err != nil {
				logger.Debugf("dial %q failed with: %v", addr, err)
				return
			}
			// No need to do the key exchange if we're already stopping
			select {
			case <-stop:
				conn.Close()
				return
			default:
			}
			// NewClientConn will close the underlying net.Conn if it gets an error
			client, _, _, err := ssh.NewClientConn(conn, addr, sshconfig)
			if err == nil {
				// We don't expect this case, because we don't support Auth,
				// but make sure to close it anyway.
				client.Close()
			}
		}(hostPort)
	}

	for finishedCount := 0; finishedCount < len(uniqueHPs); {
		select {
		case result := <-successful:
			logger.Infof("found %v has an acceptable ssh key", result)
			close(stop)
			return result, nil
		case <-finished:
			finishedCount++
		case <-time.After(timeout):
			break
		}
	}
	close(stop)
	return network.HostPort{}, errors.Errorf("cannot connect to any address: %v", hostPorts)
}
