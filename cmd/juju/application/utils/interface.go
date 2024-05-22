// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/resources"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charmresource_mock.go github.com/juju/juju/cmd/juju/application/utils CharmClient
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/resourcefacade_mock.go github.com/juju/juju/cmd/juju/application/utils ResourceLister

// CharmClient defines a subset of the charms facade, as required
// by the upgrade-charm command and to GetMetaResources.
type CharmClient interface {
	CharmInfo(string) (*charms.CharmInfo, error)
	ListCharmResources(curl string, origin apicharm.Origin) ([]charmresource.Resource, error)
}

// ResourceLister defines a subset of the resources facade, as required
// by the upgrade-charm command and to deploy bundles.
type ResourceLister interface {
	ListResources([]string) ([]resources.ApplicationResources, error)
}
