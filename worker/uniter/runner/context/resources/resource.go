// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"bytes"
	"io"

	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/docker"
)

// OpenedResourceClient exposes the API functionality needed by OpenResource.
type OpenedResourceClient interface {
	// GetResource returns the resource info and content for the given
	// name (and unit-implied application).
	GetResource(resourceName string) (resources.Resource, io.ReadCloser, error)
}

// OpenedResource wraps the resource info and reader returned
// from the API.
type OpenedResource struct {
	resources.Resource
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
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := reader.Close(); err != nil {
			return nil, errors.Trace(err)
		}
		yamlBody, err := docker.UnmarshalDockerResource(data)
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
	fp := or.Fingerprint
	// Old clients sent in the resource as json encode which means the
	// fingerprint won't match after we store the resource as yaml.
	// TODO(juju3) - remove this override
	if or.Type == charmresource.TypeContainerImage {
		fp = charmresource.Fingerprint{}
	}
	return Content{
		Data:        or.ReadCloser,
		Size:        or.Size,
		Fingerprint: fp,
	}
}
