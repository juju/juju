//go:build dqlite && linux
// +build dqlite,linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/canonical/go-dqlite"
	"github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/client"
)

// Option can be used to tweak app parameters.
type Option = app.Option

type SnapshotParams = dqlite.SnapshotParams

// WithAddress sets the network address of the application node.
//
// Other application nodes must be able to connect to this application node
// using the given address.
//
// If the application node is not the first one in the cluster, the address
// must match the value that was passed to the App.Add() method upon
// registration.
//
// If not given the first non-loopback IP address of any of the system network
// interfaces will be used, with port 9000.
//
// The address must be stable across application restarts.
func WithAddress(address string) Option {
	return app.WithAddress(address)
}

// WithCluster must be used when starting a newly added application node for
// the first time.
//
// It should contain the addresses of one or more applications nodes which are
// already part of the cluster.
func WithCluster(cluster []string) Option {
	return app.WithCluster(cluster)
}

// WithExternalConn enables passing an external dial function that will be used
// whenever dqlite needs to make an outside connection.
//
// Also takes a net.Conn channel that should be received when the external connection has been accepted.
func WithExternalConn(dialFunc client.DialFunc, acceptCh chan net.Conn) Option {
	return app.WithExternalConn(dialFunc, acceptCh)
}

// WithTLS enables TLS encryption of network traffic.
//
// The "listen" parameter must hold the TLS configuration to use when accepting
// incoming connections clients or application nodes.
//
// The "dial" parameter must hold the TLS configuration to use when
// establishing outgoing connections to other application nodes.
func WithTLS(listen *tls.Config, dial *tls.Config) Option {
	return app.WithTLS(listen, dial)
}

// WithUnixSocket allows setting a specific socket path for communication between go-dqlite and dqlite.
//
// The default is an empty string which means a random abstract unix socket.
func WithUnixSocket(path string) Option {
	return app.WithUnixSocket(path)
}

// WithVoters sets the number of nodes in the cluster that should have the
// Voter role.
//
// When a new node is added to the cluster or it is started again after a
// shutdown it will be assigned the Voter role in case the current number of
// voters is below n.
//
// Similarly when a node with the Voter role is shutdown gracefully by calling
// the Handover() method, it will try to transfer its Voter role to another
// non-Voter node, if one is available.
//
// All App instances in a cluster must be created with the same WithVoters
// setting.
//
// The given value must be an odd number greater than one.
//
// The default value is 3.
func WithVoters(n int) Option {
	return app.WithVoters(n)
}

// WithStandBys sets the number of nodes in the cluster that should have the
// StandBy role.
//
// When a new node is added to the cluster or it is started again after a
// shutdown it will be assigned the StandBy role in case there are already
// enough online voters, but the current number of stand-bys is below n.
//
// Similarly when a node with the StandBy role is shutdown gracefully by
// calling the Handover() method, it will try to transfer its StandBy role to
// another non-StandBy node, if one is available.
//
// All App instances in a cluster must be created with the same WithStandBys
// setting.
//
// The default value is 3.
func WithStandBys(n int) Option {
	return app.WithStandBys(n)
}

// WithRolesAdjustmentFrequency sets the frequency at which the current cluster
// leader will check if the roles of the various nodes in the cluster matches
// the desired setup and perform promotions/demotions to adjust the situation
// if needed.
//
// The default is 30 seconds.
func WithRolesAdjustmentFrequency(frequency time.Duration) Option {
	return app.WithRolesAdjustmentFrequency(frequency)
}

// WithLogFunc sets a custom log function.
func WithLogFunc(log client.LogFunc) Option {
	return app.WithLogFunc(log)
}

// WithFailureDomain sets the node's failure domain.
//
// Failure domains are taken into account when deciding which nodes to promote
// to Voter or StandBy when needed.
func WithFailureDomain(code uint64) Option {
	return app.WithFailureDomain(code)
}

// WithNetworkLatency sets the average one-way network latency.
func WithNetworkLatency(latency time.Duration) Option {
	return app.WithNetworkLatency(latency)
}

// WithSnapshotParams sets the raft snapshot parameters.
func WithSnapshotParams(params dqlite.SnapshotParams) Option {
	return app.WithSnapshotParams(params)
}

// App is a high-level helper for initializing a typical dqlite-based Go
// application.
//
// It takes care of starting a dqlite node and registering a dqlite Go SQL
// driver.
type App = app.App

// New creates a new application node.
func New(dir string, options ...Option) (*App, error) {
	return app.New(dir, options...)
}
