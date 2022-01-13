// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"crypto/tls"

	"github.com/juju/clock"
)

// Option can be used to tweak app parameters.
type Option func(*options)

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
	return func(options *options) {
		options.Address = address
	}
}

// WithCluster must be used when starting a newly added application node for
// the first time.
//
// It should contain the addresses of one or more applications nodes which are
// already part of the cluster.
func WithCluster(cluster []string) Option {
	return func(options *options) {
		options.Cluster = cluster
	}
}

// WithTLS enables TLS encryption of network traffic.
//
// The "listen" parameter must hold the TLS configuration to use when accepting
// incoming connections clients or application nodes.
//
// The "dial" parameter must hold the TLS configuration to use when
// establishing outgoing connections to other application nodes.
func WithTLS(listen *tls.Config, dial *tls.Config) Option {
	return func(options *options) {
		options.TLS = &tlsSetup{
			Listen: listen,
			Dial:   dial,
		}
	}
}

func WithClock(clock clock.Clock) Option {
	return func(options *options) {
		options.Clock = clock
	}
}

func WithLogger(logger Logger) Option {
	return func(options *options) {
		options.Logger = logger
	}
}

type tlsSetup struct {
	Listen *tls.Config
	Dial   *tls.Config
}

type options struct {
	Address string
	Cluster []string
	TLS     *tlsSetup
	Clock   clock.Clock
	Logger  Logger
}

func newOptions() *options {
	return &options{}
}
