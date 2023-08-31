// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"

	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/resources"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charmresource_mock.go github.com/juju/juju/cmd/juju/application/utils CharmClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resourcefacade_mock.go github.com/juju/juju/cmd/juju/application/utils ResourceLister

// CharmClient defines a subset of the charms facade, as required
// by the upgrade-charm command and to GetMetaResources.
type CharmClient interface {
	CharmInfo(string) (*charms.CharmInfo, error)
	ListCharmResources(curl *charm.URL, origin apicharm.Origin) ([]charmresource.Resource, error)
}

// ResourceLister defines a subset of the resources facade, as required
// by the upgrade-charm command and to deploy bundles.
type ResourceLister interface {
	ListResources([]string) ([]resources.ApplicationResources, error)
}
