// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"golang.org/x/net/context"

	"github.com/juju/juju/core/network"
)

// sessionEnviron implements common.ZonedEnviron. An instance of
// sessionEnviron is scoped to the context of a single exported
// method call.
//
// NOTE(axw) this type's methods are not safe for concurrent use.
// It is the responsibility of the environ type to ensure that
// there are no concurrent calls to sessionEnviron's methods.
type sessionEnviron struct {
	// environ is the environ that created this sessionEnviron.
	// Take care to synchronise access to environ's attributes
	// and methods as necessary.
	*environ

	ctx    context.Context
	client Client

	// zones caches the results of AvailabilityZones to reduce
	// the number of API calls required by StartInstance.
	// We only cache per session, so there is no issue of
	// staleness.
	zones network.AvailabilityZones
}

func (env *environ) withSession(ctx context.Context, f func(*sessionEnviron) error) error {
	return env.withClient(ctx, func(client Client) error {
		return f(&sessionEnviron{
			environ: env,
			ctx:     ctx,
			client:  client,
		})
	})
}
