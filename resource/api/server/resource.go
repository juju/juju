// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

type resourceUploader interface {
	// Upload.
	Upload(service string, name string, blob []byte) ([]resource.Resource, error)
}

type resourceFacade struct {
	uploader resourceUploader
}

// ListSpecs returns the list of resource specs for the given service.
func (f resourceFacade) Upload(args api.UploadArgs) (api.UploadResults, error) {
	var r api.UploadResults
	r.Results = make([]api.UploadResult, len(args.Entities))

	for i, e := range args.Entities {
		result, service := api.NewUploadResult(e.Tag, e.Name, e.Blob)
		r.Results[i] = result
		if result.Error != nil {
			continue
		}

		resources, err := f.uploader.Upload(e.Tag, e.Name, e.Blob)
		if err != nil {
			api.SetResultError(&r.Results[i], err)
			continue
		}

		apiResource := api.Resource2API(resource)
		r.Results[i] = apiResource
	}
	return r, nil
}
