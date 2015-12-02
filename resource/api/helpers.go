// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// ResourceSpec2API converts a resource.Spec into
// a ResourceSpec struct.
func ResourceSpec2API(r resource.Spec) ResourceSpec {
	info := r.Definition
	return ResourceSpec{
		Name:     info.Name,
		Type:     info.Type.String(),
		Path:     info.Path,
		Comment:  info.Comment,
		Origin:   r.Origin.String(),
		Revision: r.Revision,
	}
}

// API2ResourceSpec converts an API ResourceSpec info struct into
// a resource.Spec.
func API2ResourceSpec(apiSpec ResourceSpec) (resource.Spec, error) {
	var spec resource.Spec

	rtype, _ := charmresource.ParseType(apiSpec.Type)
	info := charmresource.Info{
		Name:    apiSpec.Name,
		Type:    rtype,
		Path:    apiSpec.Path,
		Comment: apiSpec.Comment,
	}
	spec.Definition = info

	origin, ok := resource.ParseOrigin(apiSpec.Origin)
	if !ok {
		return spec, errors.Trace(origin.Validate())
	}
	spec.Origin = origin

	spec.Revision = apiSpec.Revision

	if err := spec.Validate(); err != nil {
		return spec, errors.Trace(err)
	}
	return spec, nil
}
