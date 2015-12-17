// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
)

type resourcePersistence interface {
	// ListResources returns the resource data for the given service ID.
	ListResources(serviceID string) ([]resource.Resource, error)

	// SetStagedResource adds the resource in a separate staging area
	// if the resource isn't already staged. If the resource already
	// exists then it is treated as unavailable as long as the new one
	// is staged.
	SetStagedResource(serviceID string, res resource.Resource) error

	// UnstageResource ensures that the resource is removed
	// from the staging area. If it isn't in the staging area
	// then this is a noop.
	UnstageResource(serviceID, resName string) error

	// SetResource stores the resource info. If the resource
	// is already staged then it is unstaged, unless the staged
	// resource is different. In that case the request will fail.
	SetResource(serviceID string, res resource.Resource) error
}

type resourceStorage interface {
	// Put stores the content of the reader into the storage.
	Put(hash string, r io.Reader, length int64) error

	// Delete removes the identified data from the storage.
	Delete(hash string) error
}

type resourceState struct {
	persist resourcePersistence
	storage resourceStorage
}

// ListResources returns the resource data for the given service ID.
func (st resourceState) ListResources(serviceID string) ([]resource.Resource, error) {
	resources, err := st.persist.ListResources(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return resources, nil
}

// TODO(ericsnow) Separate setting the metadata from storing the blob?

// SetResource stores the resource in the Juju model.
func (st resourceState) SetResource(serviceID string, res resource.Resource, r io.Reader) error {
	if err := res.Validate(); err != nil {
		return errors.Annotate(err, "bad resource metadata")
	}
	hash := string(res.Fingerprint.Bytes())

	// TODO(ericsnow) Do something else if r is nil?

	// We use a staging approach for adding the resource metadata
	// to the model. This is necessary because the resource data
	// is stored separately and adding to both should be an atomic
	// operation.

	if err := st.persist.SetStagedResource(serviceID, res); err != nil {
		return errors.Trace(err)
	}

	if err := st.storage.Put(hash, r, res.Size); err != nil {
		if err := st.persist.UnstageResource(serviceID, res.Name); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, serviceID, err)
		}
		return errors.Trace(err)
	}

	if err := st.persist.SetResource(serviceID, res); err != nil {
		if err := st.storage.Delete(hash); err != nil {
			logger.Errorf("could not remove resource %q (service %q) from storage: %v", res.Name, serviceID, err)
		}
		if err := st.persist.UnstageResource(serviceID, res.Name); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, serviceID, err)
		}
		return errors.Trace(err)
	}

	return nil
}
