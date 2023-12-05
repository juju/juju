// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	charmresource "github.com/juju/charm/v12/resource"

	corecharm "github.com/juju/juju/core/charm"
)

// NewCharmRepository defines methods required by the resources
// facade specific to an individual repository type.
type NewCharmRepository interface {
	ResolveResources(resources []charmresource.Resource, id corecharm.CharmID) ([]charmresource.Resource, error)
}

type localClient struct{}

func (lc *localClient) ResolveResources(resources []charmresource.Resource, _ corecharm.CharmID) ([]charmresource.Resource, error) {
	var resolved []charmresource.Resource
	for _, res := range resources {
		resolved = append(resolved, charmresource.Resource{
			Meta:   res.Meta,
			Origin: charmresource.OriginUpload,
		})
	}
	return resolved, nil
}
