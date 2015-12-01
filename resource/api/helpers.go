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
	info := r.Definition()
	return ResourceSpec{
		Name:     info.Name,
		Type:     info.Type.String(),
		Path:     info.Path,
		Comment:  info.Comment,
		Origin:   r.Origin().String(),
		Revision: r.Revision(),
	}
}

// API2ResourceSpec converts an API ResourceSpec info struct into
// a resource.Spec.
func API2ResourceSpec(apiSpec ResourceSpec) (resource.Spec, error) {
	rtype, _ := charmresource.ParseType(apiSpec.Type)
	info := charmresource.Info{
		Name:    apiSpec.Name,
		Type:    rtype,
		Path:    apiSpec.Path,
		Comment: apiSpec.Comment,
	}
	// TODO(ericsnow) Call info.Validate()?

	origin, ok := resource.ParseOrigin(apiSpec.Origin)
	if !ok {
		return nil, errors.Trace(origin.Validate())
	}

	res, err := resource.NewSpec(info, origin, apiSpec.Revision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return res, nil
}
