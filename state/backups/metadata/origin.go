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
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
}

// NewOrigin returns a new backups origin.
func NewOrigin(env, machine, hostname string) *Origin {
	origin := Origin{
		Environment: env,
		Machine:     machine,
		Hostname:    hostname,
		Version:     version.Current.Number,
	}
	return &origin
}
