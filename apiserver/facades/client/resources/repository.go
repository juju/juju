// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	charmresource "github.com/juju/charm/v11/resource"
	corecharm "github.com/juju/juju/core/charm"
	coretracer "github.com/juju/juju/core/tracer"
)

// NewCharmRepository defines methods required by the resources
// facade specific to an individual repository type.
type NewCharmRepository interface {
	ResolveResources(ctx context.Context, resources []charmresource.Resource, id corecharm.CharmID) ([]charmresource.Resource, error)
}

type localClient struct{}

func (lc *localClient) ResolveResources(ctx context.Context, resources []charmresource.Resource, _ corecharm.CharmID) ([]charmresource.Resource, error) {
	_, span := coretracer.Start(ctx)
	defer span.End()

	var resolved []charmresource.Resource
	for _, res := range resources {
		resolved = append(resolved, charmresource.Resource{
			Meta:   res.Meta,
			Origin: charmresource.OriginUpload,
		})
	}
	return resolved, nil
}
