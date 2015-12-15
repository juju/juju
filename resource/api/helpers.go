// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

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

// Resource2API converts a resource.Resource into
// a Resource struct.
func Resource2API(res resource.Resource) Resource {
	return Resource{
		ResourceInfo: ResourceInfo2API(res.Info),
		Username:     res.Username,
		Timestamp:    res.Timestamp,
	}
}

// API2Resource converts an API Resource struct into
// a resource.Resource.
func API2Resource(apiRes Resource) (resource.Resource, error) {
	var res resource.Resource

	info, err := API2ResourceInfo(apiRes.ResourceInfo)
	if err != nil {
		return res, errors.Trace(err)
	}

	res = resource.Resource{
		Info:      info,
		Username:  apiRes.Username,
		Timestamp: apiRes.Timestamp,
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}

	return res, nil
}

// ResourceInfo2API converts a resource.Info into
// a ResourceInfo struct.
func ResourceInfo2API(info resource.Info) ResourceInfo {
	return ResourceInfo{
		Name:        info.Name,
		Type:        info.Type.String(),
		Path:        info.Path,
		Comment:     info.Comment,
		Revision:    info.Revision,
		Fingerprint: info.Fingerprint.Bytes(),
		Origin:      info.Origin.String(),
	}
}

// API2ResourceInfo converts an API ResourceInfo struct into
// a resource.Info.
func API2ResourceInfo(apiInfo ResourceInfo) (resource.Info, error) {
	var info resource.Info

	rtype, err := charmresource.ParseType(apiInfo.Type)
	if err != nil {
		return info, errors.Trace(err)
	}

	fp, err := charmresource.NewFingerprint(apiInfo.Fingerprint)
	if err != nil {
		return info, errors.Trace(err)
	}
	if err := fp.Validate(); err != nil {
		return info, errors.Trace(err)
	}

	origin, err := resource.ParseOriginKind(apiInfo.Origin)
	if err != nil {
		return info, errors.Trace(err)
	}

	info = resource.Info{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    apiInfo.Name,
				Type:    rtype,
				Path:    apiInfo.Path,
				Comment: apiInfo.Comment,
			},
			Revision:    apiInfo.Revision,
			Fingerprint: fp,
		},
		Origin: origin,
	}

	if err := info.Validate(); err != nil {
		return info, errors.Trace(err)
	}
	return info, nil
}
