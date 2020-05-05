// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/resource"
)

type resourcePersistence interface {
	// ListResources returns the resource data for the given application ID.
	// None of the resources will be pending.
	ListResources(applicationID string) (resource.ApplicationResources, error)

	// ListPendingResources returns the resource data for the given
	// application ID.
	ListPendingResources(applicationID string) ([]resource.Resource, error)

	// GetResource returns the extended, model-related info for the
	// non-pending resource.
	GetResource(id string) (res resource.Resource, storagePath string, _ error)

	// StageResource adds the resource in a separate staging area
	// if the resource isn't already staged. If the resource already
	// exists then it is treated as unavailable as long as the new one
	// is staged.
	StageResource(res resource.Resource, storagePath string) (*StagedResource, error)

	// SetResource stores the info for the resource.
	SetResource(args resource.Resource) error

	// SetCharmStoreResource stores the resource info that was retrieved
	// from the charm store.
	SetCharmStoreResource(id, applicationID string, res charmresource.Resource, lastPolled time.Time) error

	// SetUnitResource stores the resource info for a unit.
	SetUnitResource(unitID string, args resource.Resource) error

	// SetUnitResourceProgress stores the resource info and download
	// progressfor a unit.
	SetUnitResourceProgress(unitID string, args resource.Resource, progress int64) error

	// NewResolvePendingResourceOps generates mongo transaction operations
	// to set the identified resource as active.
	NewResolvePendingResourceOps(resID, pendingID string) ([]txn.Op, error)

	// RemovePendingAppResources removes any pending application-level
	// resources for an application. This is typically used in cleanup
	// for a failed application deployment.
	RemovePendingAppResources(applicationID string, pendingIDs map[string]string) error
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
	persist               resourcePersistence
	raw                   rawState
	dockerMetadataStorage DockerMetadataStorage
	storage               resourceStorage
	clock                 clock.Clock
}

// ListResources returns the resource data for the given application ID.
func (st resourceState) ListResources(applicationID string) (resource.ApplicationResources, error) {
	resources, err := st.persist.ListResources(applicationID)
	if err != nil {
		if err := st.raw.VerifyApplication(applicationID); err != nil {
			return resource.ApplicationResources{}, errors.Trace(err)
		}
		return resource.ApplicationResources{}, errors.Trace(err)
	}

	unitIDs, err := st.raw.Units(applicationID)
	if err != nil {
		return resource.ApplicationResources{}, errors.Trace(err)
	}
	for _, unitID := range unitIDs {
		found := false
		for _, unitRes := range resources.UnitResources {
			if unitID.String() == unitRes.Tag.String() {
				found = true
				break
			}
		}
		if !found {
			unitRes := resource.UnitResources{
				Tag: unitID,
			}
			resources.UnitResources = append(resources.UnitResources, unitRes)
		}
	}

	return resources, nil
}

// ListPendinglResources returns the resource data for the given
// application ID for pending resources only.
func (st resourceState) ListPendingResources(applicationID string) ([]resource.Resource, error) {
	resources, err := st.persist.ListPendingResources(applicationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resources, err
}

// RemovePendingResources removes the pending application-level
// resources for a specific application, normally in the case that the
// application couln't be deployed.
func (st resourceState) RemovePendingAppResources(applicationID string, pendingIDs map[string]string) error {
	return errors.Trace(st.persist.RemovePendingAppResources(applicationID, pendingIDs))
}

// GetResource returns the resource data for the identified resource.
func (st resourceState) GetResource(applicationID, name string) (resource.Resource, error) {
	id := newResourceID(applicationID, name)
	res, _, err := st.persist.GetResource(id)
	if err != nil {
		if err := st.raw.VerifyApplication(applicationID); err != nil {
			return resource.Resource{}, errors.Trace(err)
		}
		return res, errors.Trace(err)
	}
	return res, nil
}

// GetPendingResource returns the resource data for the identified resource.
func (st resourceState) GetPendingResource(applicationID, name, pendingID string) (resource.Resource, error) {
	var res resource.Resource

	resources, err := st.persist.ListPendingResources(applicationID)
	if err != nil {
		// We do not call VerifyApplication() here because pending resources
		// do not have to have an existing application.
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
func (st resourceState) SetResource(applicationID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	logger.Tracef("adding resource %q for application %q", chRes.Name, applicationID)
	pendingID := ""
	res, err := st.setResource(pendingID, applicationID, userID, chRes, r)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// SetUnitResource sets the resource metadata for a specific unit.
func (st resourceState) SetUnitResource(unitName, userID string, chRes charmresource.Resource) (_ resource.Resource, outErr error) {
	logger.Tracef("adding resource %q for unit %q", chRes.Name, unitName)
	var empty resource.Resource

	applicationID, err := names.UnitApplication(unitName)
	if err != nil {
		return empty, errors.Trace(err)
	}

	res := resource.Resource{
		Resource:      chRes,
		ID:            newResourceID(applicationID, chRes.Name),
		ApplicationID: applicationID,
	}
	res.Username = userID
	res.Timestamp = st.clock.Now().UTC()
	if err := res.Validate(); err != nil {
		return empty, errors.Annotate(err, "bad resource metadata")
	}

	if err := st.persist.SetUnitResource(unitName, res); err != nil {
		return empty, errors.Trace(err)
	}

	return res, nil
}

// AddPendingResource stores the resource in the Juju model.
func (st resourceState) AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (pendingID string, err error) {
	pendingID, err = newPendingID()
	if err != nil {
		return "", errors.Annotate(err, "could not generate resource ID")
	}
	logger.Debugf("adding pending resource %q for application %q (ID: %s)", chRes.Name, applicationID, pendingID)

	if _, err := st.setResource(pendingID, applicationID, userID, chRes, nil); err != nil {
		return "", errors.Trace(err)
	}

	return pendingID, nil
}

// UpdatePendingResource stores the resource in the Juju model.
func (st resourceState) UpdatePendingResource(applicationID, pendingID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	logger.Tracef("updating pending resource %q (%s) for application %q", chRes.Name, pendingID, applicationID)
	res, err := st.setResource(pendingID, applicationID, userID, chRes, r)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// TODO(ericsnow) Add ResolvePendingResource().

func (st resourceState) setResource(pendingID, applicationID, userID string, chRes charmresource.Resource, r io.Reader) (resource.Resource, error) {
	id := newResourceID(applicationID, chRes.Name)

	res := resource.Resource{
		Resource:      chRes,
		ID:            id,
		PendingID:     pendingID,
		ApplicationID: applicationID,
	}
	if r != nil {
		// TODO(ericsnow) Validate the user ID (or use a tag).
		res.Username = userID
		res.Timestamp = st.clock.Now().UTC()
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

	storagePath := storagePath(res.Name, res.ApplicationID, res.PendingID)
	staged, err := st.persist.StageResource(res, storagePath)
	if err != nil {
		return errors.Trace(err)
	}

	hash := res.Fingerprint.String()
	switch res.Type {
	case charmresource.TypeFile:
		err = st.storage.PutAndCheckHash(storagePath, r, res.Size, hash)
	case charmresource.TypeContainerImage:
		respBuf := new(bytes.Buffer)
		_, err = respBuf.ReadFrom(r)
		if err != nil {
			return errors.Trace(err)
		}
		dockerDetails, err := resources.UnmarshalDockerResource(respBuf.Bytes())
		if err != nil {
			return errors.Trace(err)
		}
		err = st.dockerMetadataStorage.Save(res.ID, dockerDetails)
		if err != nil {
			return errors.Trace(err)
		}
	}

	if err != nil {
		if err := staged.Unstage(); err != nil {
			logger.Errorf("could not unstage resource %q (application %q): %v", res.Name, res.ApplicationID, err)
		}
		return errors.Trace(err)
	}

	if err := staged.Activate(); err != nil {
		if err := st.storage.Remove(storagePath); err != nil {
			logger.Errorf("could not remove resource %q (application %q) from storage: %v", res.Name, res.ApplicationID, err)
		}
		if err := staged.Unstage(); err != nil {
			logger.Errorf("could not unstage resource %q (application %q): %v", res.Name, res.ApplicationID, err)
		}
		return errors.Trace(err)
	}

	return nil
}

// OpenResource returns metadata about the resource, and a reader for
// the resource.
func (st resourceState) OpenResource(applicationID, name string) (resource.Resource, io.ReadCloser, error) {
	id := newResourceID(applicationID, name)
	resourceInfo, storagePath, err := st.persist.GetResource(id)
	if err != nil {
		if err := st.raw.VerifyApplication(applicationID); err != nil {
			return resource.Resource{}, nil, errors.Trace(err)
		}
		return resource.Resource{}, nil, errors.Annotate(err, "while getting resource info")
	}
	if resourceInfo.IsPlaceholder() {
		logger.Tracef("placeholder resource %q treated as not found", name)
		return resource.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	var resourceReader io.ReadCloser
	var resSize int64
	switch resourceInfo.Type {
	case charmresource.TypeContainerImage:
		resourceReader, resSize, err = st.dockerMetadataStorage.Get(resourceInfo.ID)
	case charmresource.TypeFile:
		resourceReader, resSize, err = st.storage.Get(storagePath)
	default:
		return resource.Resource{}, nil, errors.New("unknown resource type")
	}
	if err != nil {
		return resource.Resource{}, nil, errors.Annotate(err, "while retrieving resource data")
	}
	switch resourceInfo.Type {
	case charmresource.TypeContainerImage:
		// Resource size only found at this stage in time as it's a response from the charmstore, not a stored file.
		// Store it as it's used later for verification (in a separate call than this one)
		resourceInfo.Size = resSize
		if err := st.persist.SetResource(resourceInfo); err != nil {
			return resource.Resource{}, nil, errors.Annotate(err, "failed to update resource details with docker detail size")
		}
	case charmresource.TypeFile:
		if resSize != resourceInfo.Size {
			msg := "storage returned a size (%d) which doesn't match resource metadata (%d)"
			return resource.Resource{}, nil, errors.Errorf(msg, resSize, resourceInfo.Size)
		}
	}

	return resourceInfo, resourceReader, nil
}

// OpenResourceForUniter returns metadata about the resource and
// a reader for the resource. The resource is associated with
// the unit once the reader is completely exhausted.
func (st resourceState) OpenResourceForUniter(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error) {
	applicationID := unit.ApplicationName()

	pendingID, err := newPendingID()
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	resourceInfo, resourceReader, err := st.OpenResource(applicationID, name)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	pending := resourceInfo // a copy
	pending.PendingID = pendingID

	if err := st.persist.SetUnitResourceProgress(unit.Name(), pending, 0); err != nil {
		resourceReader.Close()
		return resource.Resource{}, nil, errors.Trace(err)
	}

	resourceReader = &unitSetter{
		ReadCloser: resourceReader,
		persist:    st.persist,
		unit:       unit,
		pending:    pending,
		resource:   resourceInfo,
		clock:      clock.WallClock,
	}

	return resourceInfo, resourceReader, nil
}

// SetCharmStoreResources sets the "polled" resources for the
// application to the provided values.
func (st resourceState) SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error {
	for _, chRes := range info {
		id := newResourceID(applicationID, chRes.Name)
		if err := st.persist.SetCharmStoreResource(id, applicationID, chRes, lastPolled); err != nil {
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
func (st resourceState) NewResolvePendingResourcesOps(applicationID string, pendingIDs map[string]string) ([]txn.Op, error) {
	if len(pendingIDs) == 0 {
		return nil, nil
	}

	// TODO(ericsnow) The resources need to be pulled in from the charm
	// store before we get to this point.

	var allOps []txn.Op
	for name, pendingID := range pendingIDs {
		ops, err := st.newResolvePendingResourceOps(applicationID, name, pendingID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allOps = append(allOps, ops...)
	}
	return allOps, nil
}

func (st resourceState) newResolvePendingResourceOps(applicationID, name, pendingID string) ([]txn.Op, error) {
	resID := newResourceID(applicationID, name)
	return st.persist.NewResolvePendingResourceOps(resID, pendingID)
}

// TODO(ericsnow) Incorporate the application and resource name into the ID
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
func newResourceID(applicationID, name string) string {
	return fmt.Sprintf("%s/%s", applicationID, name)
}

// storagePath returns the path used as the location where the resource
// is stored in state storage. This requires that the returned string
// be unique and that it be organized in a structured way. In this case
// we start with a top-level (the application), then under that application use
// the "resources" section. The provided ID is located under there.
func storagePath(name, applicationID, pendingID string) string {
	// TODO(ericsnow) Use applications/<application>/resources/<resource>?
	id := name
	if pendingID != "" {
		// TODO(ericsnow) How to resolve this later?
		id += "-" + pendingID
	}
	return path.Join("application-"+applicationID, "resources", id)
}

// unitSetter records the resource as in use by a unit when the wrapped
// reader has been fully read.
type unitSetter struct {
	io.ReadCloser
	persist            resourcePersistence
	unit               resource.Unit
	pending            resource.Resource
	resource           resource.Resource
	progress           int64
	lastProgressUpdate time.Time
	clock              clock.Clock
}

// Read implements io.Reader.
func (u *unitSetter) Read(p []byte) (n int, err error) {
	n, err = u.ReadCloser.Read(p)
	if err == io.EOF {
		// record that the unit is now using this version of the resource
		if err := u.persist.SetUnitResource(u.unit.Name(), u.resource); err != nil {
			msg := "Failed to record that unit %q is using resource %q revision %v"
			logger.Errorf(msg, u.unit.Name(), u.resource.Name, u.resource.RevisionString())
		}
	} else {
		u.progress += int64(n)
		if time.Since(u.lastProgressUpdate) > time.Second {
			u.lastProgressUpdate = u.clock.Now()
			if err := u.persist.SetUnitResourceProgress(u.unit.Name(), u.pending, u.progress); err != nil {
				logger.Errorf("failed to track progress: %v", err)
			}
		}
	}
	return n, err
}
