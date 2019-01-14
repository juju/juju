// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Move this file elsewhere?
//  (e.g. top-level resource pkg, charm/resource)

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/juju/core/resources"
	"gopkg.in/httprequest.v1"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/resource"
)

// OpenedResourceClient exposes the API functionality needed by OpenResource.
type OpenedResourceClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied application).
	GetResource(resourceName string) (resource.Resource, io.ReadCloser, error)
}

// OpenedResource wraps the resource info and reader returned
// from the API.
type OpenedResource struct {
	resource.Resource
	io.ReadCloser
}

// OpenResource opens the identified resource using the provided client.
func OpenResource(name string, client OpenedResourceClient) (*OpenedResource, error) {
	info, reader, err := client.GetResource(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if info.Type == charmresource.TypeContainerImage {
		info.Path = "content.yaml"
		// Image data is stored as json but we need to convert to YAMl
		// as that's what the charm expects.
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := reader.Close(); err != nil {
			return nil, errors.Trace(err)
		}
		var yamlBody resources.DockerImageDetails
		err = json.Unmarshal(data, &yamlBody)
		if err != nil {
			return nil, errors.Trace(err)
		}
		yamlOut, err := yaml.Marshal(yamlBody)
		if err != nil {
			return nil, errors.Trace(err)
		}
		reader = httprequest.BytesReaderCloser{bytes.NewReader(yamlOut)}
		info.Size = int64(len(yamlOut))
	}
	or := &OpenedResource{
		Resource:   info,
		ReadCloser: reader,
	}
	return or, nil
}

// Content returns the "content" for the opened resource.
func (or OpenedResource) Content() Content {
	return Content{
		Data:        or.ReadCloser,
		Size:        or.Size,
		Fingerprint: or.Fingerprint,
	}
}

// Info returns the info for the opened resource.
func (or OpenedResource) Info() resource.Resource {
	return or.Resource
}
