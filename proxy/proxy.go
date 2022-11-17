// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

// Proxier describes an implemntation of an object that is capable of performing
// connection proxying. Typically an implementation will support this interface
// and one of the more specific types of proxy's below. Proxy's should be
// considered single use with regards to their Start  and Stop methods and not
// thead safe.
type Proxier interface {
	// Start starts the lifecycle of the proxy. Proxy's should have their start
	// method called before operating with the proxy.
	Start() error

	// Stop stops the proxy after a call to Start(). Proxy's should be
	// considered single use. This call should only ever be made once.
	Stop()

	// RawConfig is responsible for providing a raw configuration representation
	// of the proxier for serialising over the wire.
	RawConfig() (map[string]interface{}, error)

	// Type is the unique key identifying the type of proxying for configuration
	// purposes.
	Type() string

	// MarshalYAML implements marshalling method for yaml.
	MarshalYAML() (interface{}, error)

	// Insecure sets the proxy to be insecure.
	Insecure()
}

// TunnelProxier describes an implementation that can provide tunneled proxy
// services. The interface provides the connection details for connecting to the
// proxy
type TunnelProxier interface {
	Proxier

	// Host returns the host string for establishing a tunneled connection.
	Host() string

	// Port returns the host port to connect to for tunneling connections.
	Port() string
}
