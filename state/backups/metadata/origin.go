// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata

import (
	"github.com/juju/juju/version"
)

// Origin identifies where a backup archive came from.  While it is
// more about where and Metadata about what and when, that distinction
// does not merit special consideration.  Instead, Origin exists
// separately from Metadata due to its use as an argument when
// requesting the creation of a new backup.
type Origin struct {
	environment string
	machine     string
	hostname    string
	version     version.Number
}

// NewOrigin returns a new backups origin.
func NewOrigin(env, machine, hostname string) *Origin {
	origin := Origin{
		environment: env,
		machine:     machine,
		hostname:    hostname,
		version:     version.Current.Number,
	}
	return &origin
}

// ExistingOrigin returns a new backups origin.
func ExistingOrigin(env, machine, hostname string, vers version.Number) *Origin {
	origin := Origin{
		environment: env,
		machine:     machine,
		hostname:    hostname,
		version:     vers,
	}
	return &origin
}

// Environment is the ID for the backed-up environment.
func (o *Origin) Environment() string {
	return o.environment
}

// Machine is the ID of the state "machine" that ran the backup.
func (o *Origin) Machine() string {
	return o.machine
}

// Hostname is where the backup happened.
func (o *Origin) Hostname() string {
	return o.hostname
}

// Version is the version of juju used to produce the backup.
func (o *Origin) Version() version.Number {
	return o.version
}
