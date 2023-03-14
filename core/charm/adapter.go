// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/charm/v10"

func NewCharmInfoAdapter(meta EssentialMetadata) charmInfoAdapter {
	return charmInfoAdapter{meta: meta}
}

// charmInfoAdapter wraps an EssentialMetadata object and implements the
// charm.Charm interface so it can be passed to state.AddCharm.
type charmInfoAdapter struct {
	meta EssentialMetadata
}

func (adapter charmInfoAdapter) Meta() *charm.Meta {
	return adapter.meta.Meta
}

func (adapter charmInfoAdapter) Manifest() *charm.Manifest {
	return adapter.meta.Manifest
}

func (adapter charmInfoAdapter) Config() *charm.Config {
	return adapter.meta.Config
}

func (adapter charmInfoAdapter) LXDProfile() *charm.LXDProfile {
	return nil // not part of the essential metadata
}

func (adapter charmInfoAdapter) Metrics() *charm.Metrics {
	return nil // not part of the essential metadata
}

func (adapter charmInfoAdapter) Actions() *charm.Actions {
	return nil // not part of the essential metadata
}

func (adapter charmInfoAdapter) Revision() int {
	return 0 // not part of the essential metadata
}
