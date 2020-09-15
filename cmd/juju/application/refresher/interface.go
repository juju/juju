// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"github.com/juju/charm/v8"
	"gopkg.in/macaroon.v2"

	commoncharm "github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
)

// RefresherFactory contains a method to get a deployer.
type RefresherFactory interface {
	Run(RefresherConfig) (*CharmID, error)
}

// Refresher defines the functionality of a deployer returned by the
// factory.
type Refresher interface {
	// Refresh finishes preparing to deploy a charm or bundle,
	// then deploys it.  This is done as one step to accommodate the
	// call being wrapped by block.ProcessBlockedError.
	Refresh() (*CharmID, error)

	// String returns a string description of the deployer.
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
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin) (*charm.URL, commoncharm.Origin, []string, error)
}
