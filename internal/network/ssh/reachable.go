// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.network.ssh")

// Dialer defines a Dial() method matching the signature of net.Dial().
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// ReachableChecker tries to find ssh hosts that have a public key that matches
// our expectations.
type ReachableChecker interface {
	// FindHost tries to connect to all of the host+port combinations supplied,
	// and tries to do an SSH key negotiation. The first successful negotiation
	// that includes one of the public keys supplied will be returned. If none
	// of them can be validated, then an error will be returned.
	FindHost(hostPorts network.HostPorts, publicKeys []string) (network.HostPort, error)
}

// NewReachableChecker creates a ReachableChecker that can be used to check for
// Hosts that are viable SSH targets.
// When FindHost is called, we will dial the entries in the given hostPorts, in
// parallel, using the given dialer, closing successfully established
// connections after checking the ssh key. Individual connection errors are
// discarded, and an error is returned only if none of the hostPorts can be
// reached when the given timeout expires.
// If publicKeys is a non empty list, then the SSH host public key will be
// checked. If it is not in the list, that host is not considered valid.
//
// Usually, a net.Dialer initialized with a non-empty Timeout field is passed
// for dialer.
func NewReachableChecker(dialer Dialer, timeout time.Duration) *reachableChecker {
	return &reachableChecker{
		dialer:  dialer,
		timeout: timeout,
	}
}

// hostKeyChecker checks if this host matches one of allowed public keys
// it uses the golang/x/crypto/ssh/HostKeyCallback to find the host keys on a
// given connection.
type hostKeyChecker struct {

	// AcceptedKeys is a set of the Marshalled PublicKey content.
	AcceptedKeys set.Strings

	// Stop will be polled for whether we should stop trying to do any work.
	Stop <-chan struct{}

	// HostPort is the identifier that corresponds to this connection.
	HostPort network.HostPort

	// Accepted will be populated with a HostPort if the checker successfully
	// validated a collection.
	Accepted chan network.HostPort

	// Dialer is a Dialer that allows us to initiate the underlying TCP connection.
	Dialer Dialer

	// Finished will be set an event when we've finished our check (success or failure).
	Finished chan struct{}
}

var hostKeyNotInList = errors.New("host key not in expected set")
var hostKeyAccepted = errors.New("host key was accepted, retry")
var hostKeyAcceptedButStopped = errors.New("host key was accepted, but search was stopped")

func (h *hostKeyChecker) hostKeyCallback(ctx context.Context) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Note: we don't do any advanced checking of the PublicKey, like whether
		// the key is revoked or expired. All we care about is whether it matches
		// the public keys that we consider acceptable
		authKeyForm := ssh.MarshalAuthorizedKey(key)
		debugName := hostname
		if hostname != remote.String() {
			debugName = fmt.Sprintf("%s at %s", hostname, remote.String())
		}
		logger.Tracef(ctx, "checking host key for %s, with key %q", debugName, authKeyForm)

		lookupKey := string(key.Marshal())
		if len(h.AcceptedKeys) == 0 || h.AcceptedKeys.Contains(lookupKey) {
			logger.Debugf(ctx, "accepted host key for: %s", debugName)
			// This key was valid, so return it, but if someone else was found
			// first, still exit.
			select {
			case h.Accepted <- h.HostPort:
				// We have accepted a host, we won't need to call Finished.
				h.Finished = nil
				return hostKeyAccepted
			case <-h.Stop:
				return hostKeyAcceptedButStopped
			}
		}
		logger.Debugf(ctx, "host key for %s not in our accepted set: log at TRACE to see raw keys", debugName)
		return hostKeyNotInList
	}
}

// publicKeysToSet converts all the public key values (eg id_ed25519.pub) into
// their short hash form. Problems with a key are logged at Warning level, but
// otherwise ignored.
func publicKeysToSet(ctx context.Context, publicKeys []string) set.Strings {
	acceptedKeys := set.NewStrings()
	for _, pubKey := range publicKeys {
		// key, comment, options, rest, err
		sshKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			logger.Warningf(ctx, "unable to handle public key: %q\n", pubKey)
			continue
		}
		acceptedKeys.Add(string(sshKey.Marshal()))
	}
	return acceptedKeys
}

// Check initiates a connection to address described by the checker's HostPort
// member and tries to do an SSH key exchange to determine the preferred public
// key of the remote host.
// It then checks if that key is in the accepted set of keys.
func (h *hostKeyChecker) Check() {
	ctx := context.TODO()

	defer func() {
		// send a finished message unless we're already stopped and nobody
		// is listening
		if h.Finished != nil {
			select {
			case h.Finished <- struct{}{}:
			case <-h.Stop:
			}
		}
	}()
	// TODO(jam): 2017-01-24 One limitation of our algorithm, is that we don't
	// try to limit the negotiation of the keys to our set of possible keys.
	// For example, say we only know about the RSA key for the remote host, but
	// it has been updated to use a ECDSA key as well. Gocrypto/ssh might
	// negotiate to use the "more secure" ECDSA key and we will see that
	// as an invalid key.
	sshConfig := &ssh.ClientConfig{
		HostKeyCallback: h.hostKeyCallback(ctx),
	}
	addr := network.DialAddress(h.HostPort)
	logger.Debugf(ctx, "dialing %s to check host keys", addr)
	conn, err := h.Dialer.Dial("tcp", addr)
	if err != nil {
		logger.Debugf(ctx, "dial %s failed with: %v", addr, err)
		return
	}
	// No need to do the key exchange if we're already stopping
	select {
	case <-h.Stop:
		_ = conn.Close()
		return
	default:
	}
	logger.Debugf(ctx, "connected to %s, initiating ssh handshake", addr)
	// NewClientConn will close the underlying net.Conn if it gets an error
	client, _, _, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err == nil {
		// We don't expect this case, because we don't support Auth,
		// but make sure to close it anyway.
		_ = client.Close()
	} else {
		// no need to log these two messages, that's already been done
		// in hostKeyCallback
		if !strings.Contains(err.Error(), hostKeyAccepted.Error()) &&
			!strings.Contains(err.Error(), hostKeyNotInList.Error()) {
			logger.Debugf(ctx, "%v", err)
		}
	}
}

type reachableChecker struct {
	dialer  Dialer
	timeout time.Duration
}

// FindHost takes a list of possible host+port combinations and possible public
// keys that the SSH server could be using. We make an attempt to connect to
// each of those addresses and do an SSH handshake negotiation. We then check
// if the SSH server's negotiated public key is in our allowed set. The first
// address to successfully negotiate will be returned. If none of them succeed,
// and error will be returned.
func (r *reachableChecker) FindHost(hostPorts network.HostPorts, publicKeys []string) (network.HostPort, error) {
	uniqueHPs := hostPorts.Unique()
	successful := make(chan network.HostPort)
	stop := make(chan struct{})
	// We use a channel instead of a sync.WaitGroup so that we can return as
	// soon as we get one connected. We'll signal the rest to stop via the
	// 'stop' channel.
	finished := make(chan struct{}, len(uniqueHPs))

	acceptedKeys := publicKeysToSet(context.TODO(), publicKeys)
	for _, hostPort := range uniqueHPs {
		checker := &hostKeyChecker{
			AcceptedKeys: acceptedKeys,
			Stop:         stop,
			Accepted:     successful,
			HostPort:     hostPort,
			Dialer:       r.dialer,
			Finished:     finished,
		}
		go checker.Check()
	}

	timeout := time.After(r.timeout)
	for finishedCount := 0; finishedCount < len(uniqueHPs); {
		select {
		case result := <-successful:
			logger.Infof(context.TODO(), "found %v has an acceptable ssh key", result)
			close(stop)
			return result, nil
		case <-finished:
			finishedCount++
		case <-timeout:
			break
		}
	}
	close(stop)
	return nil, errors.Errorf("cannot connect to any address: %v", hostPorts)
}
