// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
)

// Resource2API converts a resource.Resource into
// a Resource struct.
func Resource2API(res resource.Resource) params.Resource {
	return params.Resource{
		CharmResource: CharmResource2API(res.Resource),
		ID:            res.ID,
		PendingID:     res.PendingID,
		ApplicationID: res.ApplicationID,
		Username:      res.Username,
		Timestamp:     res.Timestamp,
	}
}

// APIResult2ApplicationResources converts a ResourcesResult into a resource.ApplicationResources.
func APIResult2ApplicationResources(apiResult params.ResourcesResult) (resource.ApplicationResources, error) {
	var result resource.ApplicationResources

	if apiResult.Error != nil {
		// TODO(ericsnow) Return the resources too?
		err := common.RestoreError(apiResult.Error)
		return resource.ApplicationResources{}, errors.Trace(err)
	}

	for _, apiRes := range apiResult.Resources {
		res, err := API2Resource(apiRes)
		if err != nil {
			// This could happen if the server is misbehaving
			// or non-conforming.
			// TODO(ericsnow) Aggregate errors?
			return resource.ApplicationResources{}, errors.Annotate(err, "got bad data from server")
		}
		result.Resources = append(result.Resources, res)
	}

	for _, unitRes := range apiResult.UnitResources {
		tag, err := names.ParseUnitTag(unitRes.Tag)
		if err != nil {
			return resource.ApplicationResources{}, errors.Annotate(err, "got bad data from server")
		}
		resNames := map[string]bool{}
		unitResources := resource.UnitResources{Tag: tag}
		for _, apiRes := range unitRes.Resources {
			res, err := API2Resource(apiRes)
			if err != nil {
				return resource.ApplicationResources{}, errors.Annotate(err, "got bad data from server")
			}
			resNames[res.Name] = true
			unitResources.Resources = append(unitResources.Resources, res)
		}
		if len(unitRes.DownloadProgress) > 0 {
			unitResources.DownloadProgress = make(map[string]int64)
			for resName, progress := range unitRes.DownloadProgress {
				if _, ok := resNames[resName]; !ok {
					err := errors.Errorf("got progress from unrecognized resource %q", resName)
					return resource.ApplicationResources{}, errors.Annotate(err, "got bad data from server")
				}
				unitResources.DownloadProgress[resName] = progress
			}
		}
		result.UnitResources = append(result.UnitResources, unitResources)
	}

	for _, chRes := range apiResult.CharmStoreResources {
		res, err := API2CharmResource(chRes)
		if err != nil {
			return resource.ApplicationResources{}, errors.Annotate(err, "got bad data from server")
		}
		result.CharmStoreResources = append(result.CharmStoreResources, res)
	}

	return result, nil
}

func ApplicationResources2APIResult(svcRes resource.ApplicationResources) params.ResourcesResult {
	var result params.ResourcesResult
	for _, res := range svcRes.Resources {
		result.Resources = append(result.Resources, Resource2API(res))
	}

	for _, unitResources := range svcRes.UnitResources {
		tag := unitResources.Tag
		apiRes := params.UnitResources{
			Entity: params.Entity{Tag: tag.String()},
		}
		for _, unitRes := range unitResources.Resources {
			apiRes.Resources = append(apiRes.Resources, Resource2API(unitRes))
		}
		if len(unitResources.DownloadProgress) > 0 {
			apiRes.DownloadProgress = make(map[string]int64)
			for resName, progress := range unitResources.DownloadProgress {
				apiRes.DownloadProgress[resName] = progress
			}
		}
		result.UnitResources = append(result.UnitResources, apiRes)
	}

	result.CharmStoreResources = make([]params.CharmResource, len(svcRes.CharmStoreResources))
	for i, chRes := range svcRes.CharmStoreResources {
		result.CharmStoreResources[i] = CharmResource2API(chRes)
	}
	return result
}

// API2Resource converts an API Resource struct into
// a resource.Resource.
func API2Resource(apiRes params.Resource) (resource.Resource, error) {
	var res resource.Resource

	charmRes, err := API2CharmResource(apiRes.CharmResource)
	if err != nil {
		return res, errors.Trace(err)
	}

	res = resource.Resource{
		Resource:      charmRes,
		ID:            apiRes.ID,
		PendingID:     apiRes.PendingID,
		ApplicationID: apiRes.ApplicationID,
		Username:      apiRes.Username,
		Timestamp:     apiRes.Timestamp,
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}

	return res, nil
}

// CharmResource2API converts a charm resource into
// a CharmResource struct.
func CharmResource2API(res charmresource.Resource) params.CharmResource {
	return params.CharmResource{
		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,
		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,
	}
}

// API2CharmResource converts an API CharmResource struct into
// a charm resource.
func API2CharmResource(apiInfo params.CharmResource) (charmresource.Resource, error) {
	var res charmresource.Resource

	rtype, err := charmresource.ParseType(apiInfo.Type)
	if err != nil {
		return res, errors.Trace(err)
	}

	origin, err := charmresource.ParseOrigin(apiInfo.Origin)
	if err != nil {
		return res, errors.Trace(err)
	}

	fp, err := resource.DeserializeFingerprint(apiInfo.Fingerprint)
	if err != nil {
		return res, errors.Trace(err)
	}

	res = charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        apiInfo.Name,
			Type:        rtype,
			Path:        apiInfo.Path,
			Description: apiInfo.Description,
		},
		Origin:      origin,
		Revision:    apiInfo.Revision,
		Fingerprint: fp,
		Size:        apiInfo.Size,
	}

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}
