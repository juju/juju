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

// Resource2API converts a resource.Resource into
// a Resource struct.
func Resource2API(res resource.Resource) Resource {
	return Resource{
		CharmResource: CharmResource2API(res.Resource),
		Username:      res.Username,
		Timestamp:     res.Timestamp,
	}
}

// APIResult2Resources converts a ResourcesResult into []resource.Resource.
func APIResult2Resources(apiResult ResourcesResult) ([]resource.Resource, error) {
	var result []resource.Resource

	if apiResult.Error != nil {
		// TODO(ericsnow) Return the resources too?
		err, _ := common.RestoreError(apiResult.Error)
		return nil, errors.Trace(err)
	}

	for _, apiRes := range apiResult.Resources {
		res, err := API2Resource(apiRes)
		if err != nil {
			// This could happen if the server is misbehaving
			// or non-conforming.
			// TODO(ericsnow) Aggregate errors?
			return nil, errors.Annotate(err, "got bad data from server")
		}
		result = append(result, res)
	}

	return result, nil
}

// API2Resource converts an API Resource struct into
// a resource.Resource.
func API2Resource(apiRes Resource) (resource.Resource, error) {
	serialized := resource.Serialized{
		Name:        apiRes.Name,
		Type:        apiRes.Type,
		Path:        apiRes.Path,
		Comment:     apiRes.Comment,
		Origin:      apiRes.Origin,
		Revision:    apiRes.Revision,
		Fingerprint: apiRes.Fingerprint,
		Size:        apiRes.Size,
		Username:    apiRes.Username,
		Timestamp:   apiRes.Timestamp,
	}
	res, err := serialized.Deserialize()
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// CharmResource2API converts a charm resource into
// a CharmResource struct.
func CharmResource2API(res charmresource.Resource) CharmResource {
	serialized := resource.SerializeCharmResource(res)
	return CharmResource{
		Name:        serialized.Name,
		Type:        serialized.Type,
		Path:        serialized.Path,
		Comment:     serialized.Comment,
		Origin:      serialized.Origin,
		Revision:    serialized.Revision,
		Fingerprint: serialized.Fingerprint,
		Size:        serialized.Size,
	}
}

// API2CharmResource converts an API CharmResource struct into
// a charm resource.
func API2CharmResource(apiInfo CharmResource) (charmresource.Resource, error) {
	serialized := resource.Serialized{
		Name:        apiInfo.Name,
		Type:        apiInfo.Type,
		Path:        apiInfo.Path,
		Comment:     apiInfo.Comment,
		Origin:      apiInfo.Origin,
		Revision:    apiInfo.Revision,
		Fingerprint: apiInfo.Fingerprint,
		Size:        apiInfo.Size,
	}
	res, err := serialized.DeserializeCharm()
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}
