// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	stderrors "errors"
	"net"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"
)

// SocketTimeout should be long enough that even a slow mongo server
// will respond in that length of time, and must also be long enough
// to allow for completion of heavyweight queries.
//
// Note: 1 minute is mgo's default socket timeout value.
//
// Also note: We have observed mongodb occasionally getting "stuck"
// for over 30s in the field.
const SocketTimeout = time.Minute

// defaultDialTimeout should be representative of the upper bound of
// time taken to dial a mongo server from within the same
// cloud/private network.
const defaultDialTimeout = 30 * time.Second

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a controller.
type DialOpts struct {
	// Timeout is the amount of time to wait contacting
	// a controller.
	Timeout time.Duration

	// SocketTimeout is the amount of time to wait for a
	// non-responding socket to the database before it is forcefully
	// closed. If this is zero, the value of the SocketTimeout const
	// will be used.
	SocketTimeout time.Duration

	// Direct informs whether to establish connections only with the
	// specified seed servers, or to obtain information for the whole
	// cluster and establish connections with further servers too.
	Direct bool

	// PostDial, if non-nil, is called by DialWithInfo with the
	// mgo.Session after a successful dial but before DialWithInfo
	// returns to its caller.
	PostDial func(*mgo.Session) error

	// PostDialServer, if non-nil, is called by DialWithInfo after
	// dialing a MongoDB server connection, successfully or not.
	// The address dialed and amount of time taken are included,
	// as well as the error if any.
	PostDialServer func(addr string, _ time.Duration, _ error)

	// PoolLimit defines the per-server socket pool limit
	PoolLimit int
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a controller.
//
// NOTE(axw) these options are inappropriate for tests in CI,
// as CI tends to run on machines with slow I/O (or thrashed
// I/O with limited IOPs). For tests, use mongotest.DialOpts().
func DefaultDialOpts() DialOpts {
	return DialOpts{
		Timeout:       defaultDialTimeout,
		SocketTimeout: SocketTimeout,
	}
}

// Info encapsulates information about cluster of
// mongo servers and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state.
	// Each address should be in the form address:port.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string

	// DisableTLS controls whether the connection to MongoDB servers
	// is made using TLS (the default), or not.
	DisableTLS bool
}

// MongoInfo encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type MongoInfo struct {
	// mongo.Info contains the addresses and cert of the mongo cluster.
	Info

	// Tag holds the name of the entity that is connecting.
	// It should be nil when connecting as an administrator.
	Tag names.Tag
}

// DialInfo returns information on how to dial
// the state's mongo server with the given info
// and dial options.
func DialInfo(info Info, opts DialOpts) (*mgo.DialInfo, error) {
	if len(info.Addrs) == 0 {
		return nil, stderrors.New("no mongo addresses")
	}

	dial := func(server *mgo.ServerAddr) (_ net.Conn, err error) {
		panic("nope")
	}

	return &mgo.DialInfo{
		Addrs:      info.Addrs,
		Timeout:    opts.Timeout,
		DialServer: dial,
		Direct:     opts.Direct,
		PoolLimit:  opts.PoolLimit,
	}, nil
}

// MaybeUnauthorizedf checks if the cause of the given error is a Mongo
// authorization error, and if so, wraps the error with errors.Unauthorizedf.
func MaybeUnauthorizedf(err error, message string, args ...interface{}) error {
	if isUnauthorized(errors.Cause(err)) {
		err = errors.Unauthorizedf("unauthorized mongo access: %s", err)
	}
	return errors.Annotatef(err, message, args...)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string; and some do have error codes
	// but are not of consistent types (LastError/QueryError).
	for _, prefix := range []string{
		"auth fail",
		"not authorized",
		"server returned error on SASL authentication step: Authentication failed.",
	} {
		if strings.HasPrefix(err.Error(), prefix) {
			return true
		}
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Code == 13 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}
