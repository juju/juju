// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"github.com/juju/charm/v9"
	"gopkg.in/macaroon.v2"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
)

// RefresherFactory contains a method to get a refresher.
type RefresherFactory interface {
	Run(RefresherConfig) (*CharmID, error)
}

// Refresher defines the functionality of a refresher returned by the
// factory.
type Refresher interface {
	// Allowed will attempt to check if a refresher is allowed to run a given
	// config.
	Allowed(RefresherConfig) (bool, error)
	// Refresh a given charm. Bundles are not supported as there is no physical
	// representation in Juju.
	Refresh() (*CharmID, error)

	// String returns a string description of the refresher.
	String() string
}

// CharmID represents a charm identifier.
type CharmID struct {
	URL      *charm.URL
	Origin   corecharm.Origin
	Macaroon *macaroon.Macaroon
}

// CharmResolver defines methods required to resolve charms, as required
// by the upgrade-charm command.
type CharmResolver interface {
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error)
}

// CharmRepository defines methods for interaction with a charm repo.
type CharmRepository interface {
	// NewCharmAtPathForceSeries returns the charm represented by this path,
	// and a URL that describes it. If the series is empty,
	// the charm's default series is used, if any.
	// Otherwise, the series is validated against those the
	// charm declares it supports. If force is true, then any
	// series validation errors are ignored and the requested
	// series is used regardless. Note though that is it still
	// an error if the series is not specified and the charm does not
	// define any.
	NewCharmAtPathForceSeries(path, series string, force bool) (charm.Charm, *charm.URL, error)
}

// CommandLogger represents a logger which follows the logging
// precepts of a cmd.Context.
type CommandLogger interface {
	Infof(format string, params ...interface{})
	Warningf(format string, params ...interface{})
	Verbosef(format string, params ...interface{})
}
