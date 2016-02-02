// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type resourcePersistence interface {
	// ListResources returns the resource data for the given service ID.
	// None of the resources will be pending.
	ListResources(serviceID string) (resource.ServiceResources, error)

	// ListModelResources returns the resource data for the given
	// service ID. None of the resources will be pending.
	ListModelResources(serviceID string) ([]resource.ModelResource, error)

	// StageResource adds the resource in a separate staging area
	// if the resource isn't already staged. If the resource already
	// exists then it is treated as unavailable as long as the new one
	// is staged.
	//
	// A separate staging area is necessary because we are dealing with
	// the DB and storage at the same time for the same resource in some
	// operations (e.g. SetResource).  Resources are staged in the DB,
	// added to storage, and then finalized in the DB.
	StageResource(resource.ModelResource) error

	// UnstageResource ensures that the resource is removed
	// from the staging area. If it isn't in the staging area
	// then this is a noop.
	UnstageResource(id, serviceID string) error

	// SetResource stores the resource info. If the resource
	// is already staged then it is unstaged.
	SetResource(resource.ModelResource) error

	// SetUnitResource stores the resource info for a unit.
	SetUnitResource(unitID string, args resource.ModelResource) error
}

type resourceStorage interface {
	// PutAndCheckHash stores the content of the reader into the storage.
	PutAndCheckHash(path string, r io.Reader, length int64, hash string) error

	// Remove removes the identified data from the storage.
	Remove(path string) error

	// Get returns a reader for the resource at path. The size of the
	// data is also returned.
	Get(path string) (io.ReadCloser, int64, error)
}

type resourceState struct {
	persist resourcePersistence
	storage resourceStorage

	newPendingID     func() (string, error)
	currentTimestamp func() time.Time
}

// ListResources returns the resource data for the given service ID.
func (st resourceState) ListResources(serviceID string) (resource.ServiceResources, error) {
	resources, err := st.persist.ListResources(serviceID)
	if err != nil {
		return resource.ServiceResources{}, errors.Trace(err)
	}

	return resources, nil
}

// GetResource returns the resource data for the identified resource.
func (st resourceState) GetResource(serviceID, name string) (resource.Resource, error) {
	res, err := st.getResource(serviceID, name)
	if err != nil {
		return res.Resource, errors.Trace(err)
	}
	return res.Resource, nil
}

func (st resourceState) getResource(serviceID, name string) (resource.ModelResource, error) {
	var res resource.ModelResource

	resources, err := st.persist.ListModelResources(serviceID)
	if err != nil {
		return res, errors.Trace(err)
	}

	for _, res := range resources {
		if res.Resource.Name == name {
			return res, nil
		}
	}
	return res, errors.NotFoundf("resource %q", name)
}

// TODO(ericsnow) Separate setting the metadata from storing the blob?

// SetResource stores the resource in the Juju model.
func (st resourceState) SetResource(serviceID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	logger.Tracef("adding resource %q for service %q", chRes.Name, serviceID)
	pendingID := ""
	res, err := st.setResource(pendingID, serviceID, userID, chRes, r)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

func (st resourceState) setResource(pendingID, serviceID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	res := resource.Resource{
		Resource: chRes,
	}
	if r != nil {
		// TODO(ericsnow) Validate the user ID (or use a tag).
		res.Username = userID
		res.Timestamp = st.currentTimestamp()
	}

	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "bad resource metadata")
	}

	id := newResourceID(pendingID, serviceID, res)

	args := resource.ModelResource{
		ID:        id,
		PendingID: pendingID,
		ServiceID: serviceID,
		Resource:  res,
	}
	if r == nil {
		err := st.setResourceInfo(args)
		return res, errors.Trace(err)
	}

	// We use a staging approach for adding the resource metadata
	// to the model. This is necessary because the resource data
	// is stored separately and adding to both should be an atomic
	// operation.

	path := storagePath(res.Name, serviceID, pendingID)
	args.StoragePath = path
	if err := st.persist.StageResource(args); err != nil {
		return res, errors.Trace(err)
	}

	hash := res.Fingerprint.String()
	if err := st.storage.PutAndCheckHash(path, r, res.Size, hash); err != nil {
		if err := st.persist.UnstageResource(id, serviceID); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, serviceID, err)
		}
		return res, errors.Trace(err)
	}

	if err := st.persist.SetResource(args); err != nil {
		if err := st.storage.Remove(path); err != nil {
			logger.Errorf("could not remove resource %q (service %q) from storage: %v", res.Name, serviceID, err)
		}
		if err := st.persist.UnstageResource(id, serviceID); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, serviceID, err)
		}
		return res, errors.Trace(err)
	}

	return res, nil
}

func (st resourceState) setResourceInfo(args resource.ModelResource) error {
	if err := st.persist.StageResource(args); err != nil {
		return errors.Trace(err)
	}

	if err := st.persist.SetResource(args); err != nil {
		if err := st.persist.UnstageResource(args.ID, args.ServiceID); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", args.Resource.Name, args.ServiceID, err)
		}
		return errors.Trace(err)
	}
	return nil
}

// AddPendingResource stores the resource in the Juju model.
func (st resourceState) AddPendingResource(serviceID, userID string, chRes charmresource.Resource, r io.Reader) (pendingID string, err error) {
	pendingID, err = st.newPendingID()
	if err != nil {
		return "", errors.Annotate(err, "could not generate resource ID")
	}
	logger.Tracef("adding pending resource %q for service %q (ID: %s)", chRes.Name, serviceID, pendingID)

	if _, err := st.setResource(pendingID, serviceID, userID, chRes, r); err != nil {
		return "", errors.Trace(err)
	}

	return pendingID, nil
}

// TODO(ericsnow) Add ResolvePendingResource().

// SetUnitResource records the resource being used by a unit in the Juju model.
func (st resourceState) SetUnitResource(unit resource.Unit, res resource.Resource) error {
	logger.Tracef("adding resource %q for unit %q", res.Name, unit.Name())
	if err := res.Validate(); err != nil {
		return errors.Annotate(err, "bad resource metadata")
	}

	pendingID := ""
	serviceID := unit.ServiceName()
	id := newResourceID(pendingID, serviceID, res)
	args := resource.ModelResource{
		ID:          id,
		ServiceID:   serviceID,
		Resource:    res,
		StoragePath: storagePath(res.Name, serviceID, pendingID),
	}
	err := st.persist.SetUnitResource(unit.Name(), args)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// OpenResource returns metadata about the resource, and a reader for
// the resource.
func (st resourceState) OpenResource(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error) {
	serviceID := unit.ServiceName()

	modelResource, err := st.getResource(serviceID, name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	resourceInfo := modelResource.Resource
	if resourceInfo.IsPlaceholder() {
		return resource.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	path := modelResource.StoragePath
	resourceReader, resSize, err := st.storage.Get(path)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	if resSize != resourceInfo.Size {
		msg := "storage returned a size (%d) which doesn't match resource metadata (%d)"
		return resource.Resource{}, nil, errors.Errorf(msg, resSize, resourceInfo.Size)
	}

	id := newResourceID(modelResource.PendingID, serviceID, resourceInfo)
	resourceReader = unitSetter{
		ReadCloser: resourceReader,
		persist:    st.persist,
		unit:       unit,
		args: resource.ModelResource{
			ID:        id,
			ServiceID: serviceID,
			Resource:  resourceInfo,
		},
	}

	return resourceInfo, resourceReader, nil
}

// TODO(ericsnow) Incorporate the service and resource name into the ID
// instead of just using a UUID?

// newPendingID generates a new unique identifier for a resource.
func newPendingID() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "could not create new resource ID")
	}
	return uuid.String(), nil
}

// newResourceID produces a new ID to use for the resource in the model.
func newResourceID(pendingID, serviceID string, res resource.Resource) string {
	id := res.Name
	if pendingID != "" {
		id += "-" + pendingID
	}
	return fmt.Sprintf("service-%s/%s", serviceID, id)
}

// storagePath returns the path used as the location where the resource
// is stored in state storage. This requires that the returned string
// be unique and that it be organized in a structured way. In this case
// we start with a top-level (the service), then under that service use
// the "resources" section. The provided ID is located under there.
func storagePath(name, serviceID, pendingID string) string {
	// TODO(ericsnow) Use services/<service>/resources/<resource>?
	id := name
	if pendingID != "" {
		// TODO(ericsnow) How to resolve this later?
		id += "-" + pendingID
	}
	return path.Join("service-"+serviceID, "resources", id)
}

// unitSetter records the resource as in use by a unit when the wrapped
// reader has been fully read.
type unitSetter struct {
	io.ReadCloser
	persist resourcePersistence
	unit    resource.Unit
	args    resource.ModelResource
}

// Read implements io.Reader.
func (u unitSetter) Read(p []byte) (n int, err error) {
	n, err = u.ReadCloser.Read(p)
	if err == io.EOF {
		// record that the unit is now using this version of the resource
		if err := u.persist.SetUnitResource(u.unit.Name(), u.args); err != nil {
			msg := "Failed to record that unit %q is using resource %q revision %v"
			logger.Errorf(msg, u.unit.Name(), u.args.Resource.Name, u.args.Resource.RevisionString())
		}
	}
	return n, err
}
