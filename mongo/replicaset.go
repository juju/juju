// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"crypto/rand"
	"encoding/base64"
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/replicaset"
)

const (
	// ReplicaSetName is the name of the replica set that juju uses for
	// its state servers.
	ReplicaSetName = "juju"
)

// GenerateSharedSecret generates a pseudo-random shared secret (keyfile)
// for use with Mongo replica sets.
func GenerateSharedSecret() (string, error) {
	// "A key’s length must be between 6 and 1024 characters and may
	// only contain characters in the base64 set."
	//   -- http://docs.mongodb.org/manual/tutorial/generate-key-file/
	buf := make([]byte, base64.StdEncoding.DecodedLen(1024))
	if _, err := rand.Read(buf); err != nil {
		return "", errors.Annotate(err, "cannot read random secret")
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() []network.Address
}

// IsMaster returns a boolean that represents whether the given
// machine's peer address is the primary mongo host for the replicaset
func IsMaster(session *mgo.Session, obj WithAddresses) (bool, error) {
	addrs := obj.Addresses()

	masterHostPort, err := replicaset.MasterHostPort(session)

	// If the replica set has not been configured, then we
	// can have only one master and the caller must
	// be that master.
	if err == replicaset.ErrMasterNotConfigured {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	masterAddr, _, err := net.SplitHostPort(masterHostPort)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		if addr.Value == masterAddr {
			return true, nil
		}
	}
	return false, nil
}

// SelectPeerAddress returns the address to use as the
// mongo replica set peer address by selecting it from the given addresses.
func SelectPeerAddress(addrs []network.Address) string {
	return network.SelectInternalAddress(addrs, false)
}

// SelectPeerHostPort returns the HostPort to use as the
// mongo replica set peer by selecting it from the given hostPorts.
func SelectPeerHostPort(hostPorts []network.HostPort) string {
	return network.SelectInternalHostPort(hostPorts, false)
}
