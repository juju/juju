// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/network"
	"golang.org/x/crypto/ssh"

	"github.com/juju/loggo"
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
	acceptablePublicKeys []ssh.PublicKey
	hostPort network.HostPort
	// validHostPortChan will get hostPort passed if this host matches the
	// acceptablePublicKeys
	validHostPortChan chan network.HostPort
	// stop will be polled for whether we should stop trying to do any work
	stop <-chan struct{}
	// keyProcessed lets the setup function know when we're done
	keyProcessed chan struct{}
}

func (h *hostKeyChecker) hostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	// Note: we don't do any advanced checking of the PublicKey, like whether
	// the key is revoked or expired. All we care about is whether it matches
	// the public keys that we consider acceptable
	logger.Infof("Checking %q at %v, with key %q", hostname, remote, ssh.MarshalAuthorizedKey(key))
	defer func() {
		select {
		case h.keyProcessed <- struct{}{}:
		case <-h.stop:
		}
	}()

	for i, accepted := range h.acceptablePublicKeys {
		// TODO(jam): 2016-01-23 The ssh test suite uses Marshal and byte
		// comparision to determine a match. Is that really the best way?
		if h == nil {
			logger.Warningf("why is h nil?", i)
		}
		if accepted == nil {
			logger.Warningf("why is key %d nil?", i)
		}
		if bytes.Equal(accepted.Marshal(), key.Marshal()) {
			logger.Debugf("found match: %q %v", hostname, remote)
			select {
			case <-h.stop:
				logger.Debugf("stopped after finding a valid host: %q, %v %v", hostname, remote, h.hostPort)
				return nil
			case h.validHostPortChan <- h.hostPort:
				logger.Debugf("found valid host for: %q, %v %v", hostname, remote, h.hostPort)
				return nil
			}
		}
	}
	logger.Debugf("not the host key we were looking for %q %v", hostname, remote)
	return errors.Errorf("do not proceed")
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
	finished := make(chan struct{}, len(uniqueHPs))

	pubKeys := make([]ssh.PublicKey, len(publicKeys))
	for _, pubKey := range publicKeys {
		// key, comment, options, rest, err
		sshKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			logger.Warningf("unable to handle public key: %q\n", pubKey)
			continue
		}
		logger.Debugf("pub key: %q %v", pubKey, sshKey)
		pubKeys = append(pubKeys, sshKey)
	}
	for _, hostPort := range uniqueHPs {
		go func(hostPort network.HostPort) {
			checker := &hostKeyChecker{
				acceptablePublicKeys: pubKeys,
				hostPort: hostPort,
				validHostPortChan: successful,
				stop: stop,
				keyProcessed: make(chan struct{}, 1),
			}
			sshconfig := &ssh.ClientConfig{
				// User ?
				// Auth ?
				HostKeyCallback: checker.hostKeyCallback,
			}
			addr := hostPort.NetAddr()
			logger.Debugf("dialing %q", addr)
			conn, err := dialer.Dial("tcp", addr)
			if err != nil {
				// TODO: Tracef
				logger.Debugf("dial %q failed with: %v", addr, err)
				select {
					case <-finished:
					case <-stop:
					}
					return
			}
			logger.Debugf("NewClientConn for %q", addr)
			clientConn, chans, requests, err := ssh.NewClientConn(conn, addr, sshconfig)
			if err != nil {
				logger.Debugf("NewClientConn %q failed with: %v", addr, err)
				select {
				case <-finished:
				case <-stop:
				}
				return
			}
			logger.Debugf("Discarding requests: %q", addr)
			go ssh.DiscardRequests(requests)
			go func() {
				for ch := range chans {
					ch.Reject(ssh.ResourceShortage, "no channels allowed")
				}
			}()
			select {
			case <-checker.keyProcessed:
			case <-stop:
			}
			clientConn.Close()
		}(hostPort)
	}

	for finishedCount := 0; finishedCount < len(uniqueHPs); {
		select {
		case result := <-successful:
			logger.Infof("dialed %q successfully", result)
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
