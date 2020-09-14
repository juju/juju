// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"gopkg.in/macaroon.v2"

	corecharm "github.com/juju/juju/core/charm"
)

// RefresherFactory contains a method to get a deployer.
type RefresherFactory interface {
	GetRefresher(RefresherConfig) (Refresher, error)
}

// Refresher defines the functionality of a deployer returned by the
// factory.
type Refresher interface {
	// PrepareAndRefresh finishes preparing to deploy a charm or bundle,
	// then deploys it.  This is done as one step to accommodate the
	// call being wrapped by block.ProcessBlockedError.
	PrepareAndRefresh(*cmd.Context) (*charm.URL, corecharm.Origin, *macaroon.Macaroon, error)

	// String returns a string description of the deployer.
	String() string
}
