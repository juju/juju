// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// TODO(ericsnow) Figure out a way to drop the txn dependency here?

import (
	"fmt"
	"io"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

type resourcePersistence interface {
	// ListResources returns the resource data for the given service ID.
	// None of the resources will be pending.
	ListResources(serviceID string) (resource.ServiceResources, error)

	// ListPendingResources returns the resource data for the given
	// service ID.
	ListPendingResources(serviceID string) ([]resource.Resource, error)

	// GetResource returns the extended, model-related info for the
	// non-pending resource.
	GetResource(id string) (res resource.Resource, storagePath string, _ error)

	// StageResource adds the resource in a separate staging area
	// if the resource isn't already staged. If the resource already
	// exists then it is treated as unavailable as long as the new one
	// is staged.
	StageResource(res resource.Resource, storagePath string) (StagedResource, error)

	// SetResource stores the info for the resource.
	SetResource(args resource.Resource) error

	// SetCharmStoreResource stores the resource info that was retrieved
	// from the charm store.
	SetCharmStoreResource(id, serviceID string, res charmresource.Resource, lastPolled time.Time) error

	// SetUnitResource stores the resource info for a unit.
	SetUnitResource(unitID string, args resource.Resource) error

	// NewResolvePendingResourceOps generates mongo transaction operations
	// to set the identified resource as active.
	NewResolvePendingResourceOps(resID, pendingID string) ([]txn.Op, error)
}

// StagedResource represents resource info that has been added to the
// "staging" area of the persistence layer.
//
// A separate staging area is necessary because we are dealing with
// the DB and storage at the same time for the same resource in some
// operations (e.g. SetResource).  Resources are staged in the DB,
// added to storage, and then finalized in the DB.
type StagedResource interface {
	// Unstage ensures that the resource is removed
	// from the staging area. If it isn't in the staging area
	// then this is a noop.
	Unstage() error

	// Activate makes the staged resource the active resource.
	Activate() error
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
	id := newResourceID(serviceID, name)
	res, _, err := st.persist.GetResource(id)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// GetPendingResource returns the resource data for the identified resource.
func (st resourceState) GetPendingResource(serviceID, name, pendingID string) (resource.Resource, error) {
	var res resource.Resource

	resources, err := st.persist.ListPendingResources(serviceID)
	if err != nil {
		return res, errors.Trace(err)
	}

	for _, res := range resources {
		if res.Name == name && res.PendingID == pendingID {
			return res, nil
		}
	}
	return res, errors.NotFoundf("pending resource %q (%s)", name, pendingID)
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

// UpdatePendingResource stores the resource in the Juju model.
func (st resourceState) UpdatePendingResource(serviceID, pendingID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	logger.Tracef("updating pending resource %q (%s) for service %q", chRes.Name, pendingID, serviceID)
	res, err := st.setResource(pendingID, serviceID, userID, chRes, r)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// TODO(ericsnow) Add ResolvePendingResource().

func (st resourceState) setResource(pendingID, serviceID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	id := newResourceID(serviceID, chRes.Name)

	res := resource.Resource{
		Resource:  chRes,
		ID:        id,
		PendingID: pendingID,
		ServiceID: serviceID,
	}
	if r != nil {
		// TODO(ericsnow) Validate the user ID (or use a tag).
		res.Username = userID
		res.Timestamp = st.currentTimestamp()
	}

	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "bad resource metadata")
	}

	if r == nil {
		if err := st.persist.SetResource(res); err != nil {
			return res, errors.Trace(err)
		}
	} else {
		if err := st.storeResource(res, r); err != nil {
			return res, errors.Trace(err)
		}
	}

	return res, nil
}

func (st resourceState) storeResource(res resource.Resource, r io.Reader) error {
	// We use a staging approach for adding the resource metadata
	// to the model. This is necessary because the resource data
	// is stored separately and adding to both should be an atomic
	// operation.

	storagePath := storagePath(res.Name, res.ServiceID, res.PendingID)
	staged, err := st.persist.StageResource(res, storagePath)
	if err != nil {
		return errors.Trace(err)
	}

	hash := res.Fingerprint.String()
	if err := st.storage.PutAndCheckHash(storagePath, r, res.Size, hash); err != nil {
		if err := staged.Unstage(); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, res.ServiceID, err)
		}
		return errors.Trace(err)
	}

	if err := staged.Activate(); err != nil {
		if err := st.storage.Remove(storagePath); err != nil {
			logger.Errorf("could not remove resource %q (service %q) from storage: %v", res.Name, res.ServiceID, err)
		}
		if err := staged.Unstage(); err != nil {
			logger.Errorf("could not unstage resource %q (service %q): %v", res.Name, res.ServiceID, err)
		}
		return errors.Trace(err)
	}

	return nil
}

// OpenResource returns metadata about the resource, and a reader for
// the resource.
func (st resourceState) OpenResource(serviceID, name string) (resource.Resource, io.ReadCloser, error) {
	id := newResourceID(serviceID, name)
	resourceInfo, storagePath, err := st.persist.GetResource(id)
	if err != nil {
		return resource.Resource{}, nil, errors.Annotate(err, "while getting resource info")
	}
	if resourceInfo.IsPlaceholder() {
		logger.Tracef("placeholder resource %q treated as not found", name)
		return resource.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	resourceReader, resSize, err := st.storage.Get(storagePath)
	if err != nil {
		return resource.Resource{}, nil, errors.Annotate(err, "while retrieving resource data")
	}
	if resSize != resourceInfo.Size {
		msg := "storage returned a size (%d) which doesn't match resource metadata (%d)"
		return resource.Resource{}, nil, errors.Errorf(msg, resSize, resourceInfo.Size)
	}

	return resourceInfo, resourceReader, nil
}

// OpenResourceForUniter returns metadata about the resource and
// a reader for the resource. The resource is associated with
// the unit once the reader is completely exhausted.
func (st resourceState) OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error) {
	serviceID := unit.ServiceName()

	resourceInfo, resourceReader, err := st.OpenResource(serviceID, name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	resourceReader = unitSetter{
		ReadCloser: resourceReader,
		persist:    st.persist,
		unit:       unit,
		resource:   resourceInfo,
	}

	return resourceInfo, resourceReader, nil
}

// SetCharmStoreResources sets the "polled" resources for the
// service to the provided values.
func (st resourceState) SetCharmStoreResources(serviceID string, info []charmresource.Resource, lastPolled time.Time) error {
	for _, chRes := range info {
		id := newResourceID(serviceID, chRes.Name)
		if err := st.persist.SetCharmStoreResource(id, serviceID, chRes, lastPolled); err != nil {
			return errors.Trace(err)
		}
		// TODO(ericsnow) Worry about extras? missing?
	}

	return nil
}

// TODO(ericsnow) Rename NewResolvePendingResourcesOps to reflect that
// it has more meat to it?

// NewResolvePendingResourcesOps generates mongo transaction operations
// to set the identified resources as active.
//
// Leaking mongo details (transaction ops) is a necessary evil since we
// do not have any machinery to facilitate transactions between
// different components.
func (st resourceState) NewResolvePendingResourcesOps(serviceID string, pendingIDs map[string]string) ([]txn.Op, error) {
	if len(pendingIDs) == 0 {
		return nil, nil
	}

	// TODO(ericsnow) The resources need to be pulled in from the charm
	// store before we get to this point.

	var allOps []txn.Op
	for name, pendingID := range pendingIDs {
		ops, err := st.newResolvePendingResourceOps(serviceID, name, pendingID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allOps = append(allOps, ops...)
	}
	return allOps, nil
}

func (st resourceState) newResolvePendingResourceOps(serviceID, name, pendingID string) ([]txn.Op, error) {
	resID := newResourceID(serviceID, name)
	return st.persist.NewResolvePendingResourceOps(resID, pendingID)
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
func newResourceID(serviceID, name string) string {
	return fmt.Sprintf("%s/%s", serviceID, name)
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
	persist  resourcePersistence
	unit     resource.Unit
	resource resource.Resource
}

// Read implements io.Reader.
func (u unitSetter) Read(p []byte) (n int, err error) {
	n, err = u.ReadCloser.Read(p)
	if err == io.EOF {
		// record that the unit is now using this version of the resource
		if err := u.persist.SetUnitResource(u.unit.Name(), u.resource); err != nil {
			msg := "Failed to record that unit %q is using resource %q revision %v"
			logger.Errorf(msg, u.unit.Name(), u.resource.Name, u.resource.RevisionString())
		}
	}
	return n, err
}
