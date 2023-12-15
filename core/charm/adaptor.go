// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import "github.com/juju/charm/v12"

func NewCharmInfoAdaptor(meta EssentialMetadata) charmInfoAdaptor {
	return charmInfoAdaptor{meta: meta}
}

// charmInfoAdaptor wraps an EssentialMetadata object and implements the
// charm.Charm interface so it can be passed to state.AddCharm.
type charmInfoAdaptor struct {
	meta EssentialMetadata
}

func (adaptor charmInfoAdaptor) Meta() *charm.Meta {
	return adaptor.meta.Meta
}

func (adaptor charmInfoAdaptor) Manifest() *charm.Manifest {
	return adaptor.meta.Manifest
}

func (adaptor charmInfoAdaptor) Config() *charm.Config {
	return adaptor.meta.Config
}

func (adaptor charmInfoAdaptor) LXDProfile() *charm.LXDProfile {
	return nil // not part of the essential metadata
}

func (adaptor charmInfoAdaptor) Metrics() *charm.Metrics {
	return nil // not part of the essential metadata
}

func (adaptor charmInfoAdaptor) Actions() *charm.Actions {
	return nil // not part of the essential metadata
}

func (adaptor charmInfoAdaptor) Revision() int {
	return 0 // not part of the essential metadata
}
