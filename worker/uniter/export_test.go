// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import "github.com/juju/juju/worker/uniter/resolver"

type ResolverConfig resolverConfig

// NewUniterResolver returns a new aggregate uniter resolver.
func NewUniterResolver(cfg ResolverConfig) resolver.Resolver {
	return newUniterResolver(resolverConfig(cfg))
}
