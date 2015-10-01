// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

// Config contains the config values used for a connection to the LXD API.
type Config struct {
	// Namespace identifies the namespace to associate with containers
	// and other resources with which the client interacts. If may be
	// blank.
	Namespace string

	// Remote identifies the host to which the client should connect.
	// An empty string is interpreted as:
	//   "localhost over a unix socket (unencrypted)".
	Remote string
}

// Validate checks the client's fields for invalid values.
func (cfg Config) Validate() error {
	return nil
}
