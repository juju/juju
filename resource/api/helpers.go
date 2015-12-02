// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the dependence on apiserver if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/resource"
)

// ServiceTag2ID converts the provided tag into a service ID.
func ServiceTag2ID(tagStr string) (string, error) {
	kind, err := names.TagKind(tagStr)
	if err != nil {
		return "", errors.Annotatef(err, "could not determine tag type from %q", tagStr)
	}
	if kind != names.ServiceTagKind {
		return "", errors.Errorf("expected service tag, got %q", tagStr)
	}

	tag, err := names.ParseTag(tagStr)
	if err != nil {
		return "", errors.Errorf("invalid service tag %q", tagStr)
	}
	return tag.Id(), nil
}

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
		Revision: r.Revision.String(),
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

	rev, ok := resource.ParseRevision(apiSpec.Revision)
	if !ok {
		return spec, errors.Trace(origin.Validate())
	}
	spec.Revision = rev

	if err := spec.Validate(); err != nil {
		return spec, errors.Trace(err)
	}
	return spec, nil
}

// API2SpecsResult converts a ResourceSpecsResult into a resource.SpecsResult.
func API2SpecsResult(service string, apiResult ResourceSpecsResult) (resource.SpecsResult, error) {
	result := resource.SpecsResult{
		Service: service,
	}

	result.Error, _ = common.RestoreError(apiResult.Error)

	var failure error
	for _, apiSpec := range apiResult.Specs {
		spec, err := API2ResourceSpec(apiSpec)
		if err != nil {
			// This could happen if the server is misbehaving
			// or non-conforming.
			if result.Error == nil {
				result.Error = errors.Annotate(err, "got bad data from server")
				failure = result.Error
			}
			// TODO(ericsnow) Set an empty spec?
		}
		result.Specs = append(result.Specs, spec)
	}

	return result, failure
}
