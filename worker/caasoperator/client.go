// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// Client provides an interface for interacting
// with the CAASOperator API. Subsets of this
// should be passed to the CAASOperator worker.
type Client interface {
	ApplicationConfigGetter
	CharmGetter
	StatusSetter
}

// CharmGetter provides an interface for getting
// the URL and SHA256 hash of the charm currently
// assigned to the application.
type CharmGetter interface {
	Charm(application string) (_ *charm.URL, sha256 string, _ error)
}

// StatusSetter provides an interface for setting
// the status of a CAAS application.
type StatusSetter interface {
	// SetStatus sets the status of an application.
	SetStatus(
		application string,
		status status.Status,
		info string,
		data map[string]interface{},
	) error
}

// ApplicationConfigGetter provides an interface for
// watching and getting the application's config settings.
type ApplicationConfigGetter interface {
	ApplicationConfig(string) (charm.Settings, error)
	WatchApplicationConfig(string) (watcher.NotifyWatcher, error)
}
