// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
)

// ResourceSpec2API converts a resource.ResourceSpec into
// a ResourceSpec struct.
func ResourceSpec2API(r resource.ResourceSpec) ResourceSpec {
	info := r.Definition()
	return ResourceSpec{
		Name:     info.Name,
		Type:     info.Type,
		Path:     info.Path,
		Comment:  info.Comment,
		Origin:   r.Origin(),
		Revision: r.Revision(),
	}
}

// API2ResourceSpec converts an API ResourceSpec info struct into
// a resource.ResourceSpec.
func API2ResourceSpec(apiSpec ResourceSpec) (resource.ResourceSpec, error) {
	info := charm.ResourceInfo{
		Name:    apiSpec.Name,
		Type:    apiSpec.Type,
		Path:    apiSpec.Path,
		Comment: apiSpec.Comment,
	}
	res, err := resource.NewResourceSpec(info, apiSpec.Origin, apiSpec.Revision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return res, nil
}
