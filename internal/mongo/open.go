// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	stderrors "errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/cert"

	"github.com/juju/juju/internal/http"
)

// MongoPassword is the password used whilst we're getting rid of mongo.
// This is a temporary measure to allow us to connect to mongo
// without having to set up a password in the mongo configuration.
const MongoPassword = "deadbeef"

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

	var tlsConfig *tls.Config
	if !info.DisableTLS {
		if len(info.CACert) == 0 {
			return nil, stderrors.New("missing CA certificate")
		}
		xcert, err := cert.ParseCert(info.CACert)
		if err != nil {
			return nil, fmt.Errorf("cannot parse CA certificate: %v", err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(xcert)

		tlsConfig = http.SecureTLSConfig()
		tlsConfig.RootCAs = pool
		tlsConfig.ServerName = "juju-mongodb"

		// TODO(natefinch): revisit this when are full-time on mongo 3.
		// We have to add non-ECDHE suites because mongo doesn't support ECDHE.
		moreSuites := []uint16{
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		}
		tlsConfig.CipherSuites = append(tlsConfig.CipherSuites, moreSuites...)
	}

	dial := func(server *mgo.ServerAddr) (_ net.Conn, err error) {
		if opts.PostDialServer != nil {
			before := time.Now()
			defer func() {
				taken := time.Now().Sub(before)
				opts.PostDialServer(server.String(), taken, err)
			}()
		}

		addr := server.TCPAddr().String()
		c, err := net.DialTimeout("tcp", addr, opts.Timeout)
		if err != nil {
			logger.Debugf(context.TODO(), "mongodb connection failed, will retry: %v", err)
			return nil, err
		}
		if tlsConfig != nil {
			cc := tls.Client(c, tlsConfig)
			if err := cc.Handshake(); err != nil {
				logger.Warningf(context.TODO(), "TLS handshake failed: %v", err)
				if err := c.Close(); err != nil {
					logger.Warningf(context.TODO(), "failed to close connection: %v", err)
				}
				return nil, err
			}
			c = cc
		}
		logger.Debugf(context.TODO(), "dialed mongodb server at %q", addr)
		return c, nil
	}

	return &mgo.DialInfo{
		Addrs:      info.Addrs,
		Timeout:    opts.Timeout,
		DialServer: dial,
		Direct:     opts.Direct,
		PoolLimit:  opts.PoolLimit,
	}, nil
}

// DialWithInfo establishes a new session to the cluster identified by info,
// with the specified options. If either Tag or Password are specified, then
// a Login call on the admin database will be made.
func DialWithInfo(info MongoInfo, opts DialOpts) (*mgo.Session, error) {
	if opts.Timeout == 0 {
		return nil, errors.New("a non-zero Timeout must be specified")
	}

	dialInfo, err := DialInfo(info.Info, opts)
	if err != nil {
		return nil, err
	}

	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}

	if opts.SocketTimeout == 0 {
		opts.SocketTimeout = SocketTimeout
	}
	session.SetSocketTimeout(opts.SocketTimeout)

	if opts.PostDial != nil {
		if err := opts.PostDial(session); err != nil {
			session.Close()
			return nil, errors.Annotate(err, "PostDial failed")
		}
	}
	if info.Tag != nil {
		user := AdminUser
		if info.Tag != nil {
			user = info.Tag.String()
		}
		if err := Login(session, user); err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
	}
	return session, nil
}

// Login logs in to the mongodb admin database.
func Login(session *mgo.Session, user string) error {
	admin := session.DB("admin")
	if err := admin.Login(user, MongoPassword); err != nil {
		return MaybeUnauthorizedf(err, "cannot log in to admin database as %q", user)
	}
	return nil
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
