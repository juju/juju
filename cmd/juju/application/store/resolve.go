// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.juju.application.store")

// ResolvedBundle decorates a charm.Bundle instance with a type that implements
// the charm.BundleDataSource interface.
type ResolvedBundle struct {
	parts []*charm.BundleDataPart
}

func NewResolvedBundle(b charm.Bundle) ResolvedBundle {
	return ResolvedBundle{
		parts: []*charm.BundleDataPart{
			{
				Data:        b.Data(),
				PresenceMap: make(charm.FieldPresenceMap),
			},
		},
	}
}

// Parts implements charm.BundleDataSource.
func (rb ResolvedBundle) Parts() []*charm.BundleDataPart {
	return rb.parts
}

// BasePath implements charm.BundleDataSource.
func (ResolvedBundle) BasePath() string {
	return ""
}

// ResolveInclude implements charm.BundleDataSource.
func (ResolvedBundle) ResolveInclude(_ string) ([]byte, error) {
	return nil, errors.NotSupportedf("remote bundle includes")
}
