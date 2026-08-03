// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/internal/worker/apicaller"
)

// NewWorker creates a firewaller worker.
func NewWorker(cfg Config) (worker.Worker, error) {
	w, err := NewFirewaller(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// crossmodelFirewallerFacadeFunc returns a function that
// can be used to construct instances which manage remote relation
// firewall changes for a given model.

// For now we use a facade, but in future this may evolve into a REST caller.
func crossmodelFirewallerFacadeFunc(
	connectionFunc apicaller.NewExternalControllerConnectionFunc,
) newCrossModelFacadeFunc {
	return func(ctx context.Context, apiInfo *api.Info) (CrossModelFirewallerFacadeCloser, error) {
		apiInfo.Tag = names.NewUserTag(api.AnonymousUsername)
		conn, err := connectionFunc(ctx, apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodelrelations.NewClient(conn), nil
	}
}
