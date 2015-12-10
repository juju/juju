// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type rawSpecState interface {
	// CharmMetadata returns the charm metadata for the identified service.
	CharmMetadata(serviceID string) (*charm.Meta, error)
}

type specState struct {
	raw rawSpecState
}

// ListResourceSpecs returns the resource specs for the given service ID.
func (st specState) ListResourceSpecs(serviceID string) ([]resource.Spec, error) {
	meta, err := st.raw.CharmMetadata(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var specs []resource.Spec
	for _, res := range meta.Resources {
		spec, err := newSpec(res, serviceID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func newSpec(res charmresource.Resource, serviceID string) (resource.Spec, error) {
	// TODO(ericsnow) For now uploads are the only supported origin.
	// Once that changes, this code will need to adjust.
	spec := resource.Spec{
		Definition: res.Info,
		Origin:     resource.OriginKindUpload,
		Revision:   resource.NoRevision,
	}
	if err := spec.Validate(); err != nil {
		return spec, errors.Annotatef(err, "invalid charm metadata for service %q", serviceID)
	}
	return spec, nil
}
