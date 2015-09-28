// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import "github.com/juju/juju/worker/uniter/operation"

type ResolverOpFactory struct {
	*resolverOpFactory
}

func NewResolverOpFactory(f operation.Factory) ResolverOpFactory {
	return ResolverOpFactory{&resolverOpFactory{
		Factory:    f,
		LocalState: &LocalState{},
	}}
}

var UpdateCharmDir = updateCharmDir
