// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

const (
	resourcesC = "resources"

	resourcesStagedIDSuffix     = "#staged"
	resourcesCharmstoreIDSuffix = "#charmstore"
)

// resourceID converts an external resource ID into an internal one.
func resourceID(id, subType, subID string) string {
	if subType == "" {
		return fmt.Sprintf("resource#%s", id)
	}
	return fmt.Sprintf("resource#%s#%s-%s", id, subType, subID)
}

func serviceResourceID(id string) string {
	return resourceID(id, "", "")
}

func pendingResourceID(id, pendingID string) string {
	return resourceID(id, "pending", pendingID)
}

func charmStoreResourceID(id string) string {
	return serviceResourceID(id) + resourcesCharmstoreIDSuffix
}

func unitResourceID(id, unitID string) string {
	return resourceID(id, "unit", unitID)
}

// stagedResourceID converts an external resource ID into an internal
// staged one.
func stagedResourceID(id string) string {
	return serviceResourceID(id) + resourcesStagedIDSuffix
}

// storedResource holds all model-stored information for a resource.
type storedResource struct {
	resource.Resource

	// storagePath is the path to where the resource content is stored.
	storagePath string
}

// charmStoreResource holds the info for a resource as provided by the
// charm store at as specific point in time.
type charmStoreResource struct {
	charmresource.Resource
	id         string
	serviceID  string
	lastPolled time.Time
}

func newInsertStagedResourceOps(stored storedResource) []txn.Op {
	doc := newStagedResourceDoc(stored)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newEnsureStagedResourceSameOps(stored storedResource) []txn.Op {
	doc := newStagedResourceDoc(stored)

	// Other than cause the txn to abort, we don't do anything here.
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: doc, // TODO(ericsnow) Is this okay?
	}}
}

func newRemoveStagedResourceOps(id string) []txn.Op {
	fullID := stagedResourceID(id)

	// We don't assert that it exists. We want "missing" to be a noop.
	return []txn.Op{{
		C:      resourcesC,
		Id:     fullID,
		Remove: true,
	}}
}

func newInsertResourceOps(stored storedResource) []txn.Op {
	doc := newResourceDoc(stored)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateResourceOps(stored storedResource) []txn.Op {
	doc := newResourceDoc(stored)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, newInsertResourceOps(stored)...)
}

func newInsertCharmStoreResourceOps(res charmStoreResource) []txn.Op {
	doc := newCharmStoreResourceDoc(res)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateCharmStoreResourceOps(res charmStoreResource) []txn.Op {
	doc := newCharmStoreResourceDoc(res)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, newInsertCharmStoreResourceOps(res)...)
}

func newInsertUnitResourceOps(unitID string, stored storedResource) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateUnitResourceOps(unitID string, stored storedResource) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, newInsertUnitResourceOps(unitID, stored)...)
}

// newResolvePendingResourceOps generates transaction operations that
// will resolve a pending resource doc and make it active.
//
// We trust that the provided resource really is pending
// and that it matches the existing doc with the same ID.
func newResolvePendingResourceOps(pending storedResource, exists bool) []txn.Op {
	oldID := pendingResourceID(pending.ID, pending.PendingID)
	newRes := pending
	newRes.PendingID = ""
	// TODO(ericsnow) Update newRes.StoragePath? Doing so would require
	// moving the resource in the blobstore to the correct path, which
	// we cannot do in the transaction...
	ops := []txn.Op{{
		C:      resourcesC,
		Id:     oldID,
		Assert: txn.DocExists,
		Remove: true,
	}}
	if exists {
		return append(ops, newUpdateResourceOps(newRes)...)
	} else {
		return append(ops, newInsertResourceOps(newRes)...)
	}
}

// newCharmStoreResourceDoc generates a doc that represents the given resource.
func newCharmStoreResourceDoc(res charmStoreResource) *resourceDoc {
	fullID := charmStoreResourceID(res.id)
	return charmStoreResource2Doc(fullID, res)
}

// newUnitResourceDoc generates a doc that represents the given resource.
func newUnitResourceDoc(unitID string, stored storedResource) *resourceDoc {
	fullID := unitResourceID(stored.ID, unitID)
	return unitResource2Doc(fullID, unitID, stored)
}

// newResourceDoc generates a doc that represents the given resource.
func newResourceDoc(stored storedResource) *resourceDoc {
	fullID := serviceResourceID(stored.ID)
	if stored.PendingID != "" {
		fullID = pendingResourceID(stored.ID, stored.PendingID)
	}
	return resource2doc(fullID, stored)
}

// newStagedResourceDoc generates a staging doc that represents
// the given resource.
func newStagedResourceDoc(stored storedResource) *resourceDoc {
	stagedID := stagedResourceID(stored.ID)
	return resource2doc(stagedID, stored)
}

// resources returns the resource docs for the given service.
func (p ResourcePersistence) resources(serviceID string) ([]resourceDoc, error) {
	logger.Tracef("querying db for resources for %q", serviceID)
	var docs []resourceDoc
	query := bson.D{{"service-id", serviceID}}
	if err := p.base.All(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("found %d resources", len(docs))
	return docs, nil
}

// getOne returns the resource that matches the provided model ID.
func (p ResourcePersistence) getOne(resID string) (resourceDoc, error) {
	logger.Tracef("querying db for resource %q", resID)
	id := serviceResourceID(resID)
	var doc resourceDoc
	if err := p.base.One(resourcesC, id, &doc); err != nil {
		return doc, errors.Trace(err)
	}
	return doc, nil
}

// getOnePending returns the resource that matches the provided model ID.
func (p ResourcePersistence) getOnePending(resID, pendingID string) (resourceDoc, error) {
	logger.Tracef("querying db for resource %q (pending %q)", resID, pendingID)
	id := pendingResourceID(resID, pendingID)
	var doc resourceDoc
	if err := p.base.One(resourcesC, id, &doc); err != nil {
		return doc, errors.Trace(err)
	}
	return doc, nil
}

// resourceDoc is the top-level document for resources.
type resourceDoc struct {
	DocID     string `bson:"_id"`
	EnvUUID   string `bson:"env-uuid"`
	ID        string `bson:"resource-id"`
	PendingID string `bson:"pending-id"`

	ServiceID string `bson:"service-id"`
	UnitID    string `bson:"unit-id"`

	Name        string `bson:"name"`
	Type        string `bson:"type"`
	Path        string `bson:"path"`
	Description string `bson:"description"`

	Origin      string `bson:"origin"`
	Revision    int    `bson:"revision"`
	Fingerprint []byte `bson:"fingerprint"`
	Size        int64  `bson:"size"`

	Username  string    `bson:"username"`
	Timestamp time.Time `bson:"timestamp-when-added"`

	StoragePath string `bson:"storage-path"`

	LastPolled time.Time `bson:"timestamp-when-last-polled"`
}

func charmStoreResource2Doc(id string, res charmStoreResource) *resourceDoc {
	stored := storedResource{
		Resource: resource.Resource{
			Resource:  res.Resource,
			ID:        res.id,
			ServiceID: res.serviceID,
		},
	}
	doc := resource2doc(id, stored)
	doc.LastPolled = res.lastPolled
	return doc
}

func unitResource2Doc(id, unitID string, stored storedResource) *resourceDoc {
	doc := resource2doc(id, stored)
	doc.UnitID = unitID
	return doc
}

// resource2doc converts the resource into a DB doc.
func resource2doc(id string, stored storedResource) *resourceDoc {
	res := stored.Resource
	// TODO(ericsnow) We may need to limit the resolution of timestamps
	// in order to avoid some conversion problems from Mongo.
	return &resourceDoc{
		DocID:     id,
		ID:        res.ID,
		PendingID: res.PendingID,

		ServiceID: res.ServiceID,

		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,

		Username:  res.Username,
		Timestamp: res.Timestamp,

		StoragePath: stored.storagePath,
	}
}

// doc2resource returns the resource info represented by the doc.
func doc2resource(doc resourceDoc) (storedResource, error) {
	res, err := doc2basicResource(doc)
	if err != nil {
		return storedResource{}, errors.Trace(err)
	}

	stored := storedResource{
		Resource:    res,
		storagePath: doc.StoragePath,
	}
	return stored, nil
}

// doc2basicResource returns the resource info represented by the doc.
func doc2basicResource(doc resourceDoc) (resource.Resource, error) {
	var res resource.Resource

	resType, err := charmresource.ParseType(doc.Type)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	origin, err := charmresource.ParseOrigin(doc.Origin)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	fp, err := resource.DeserializeFingerprint(doc.Fingerprint)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	res = resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        doc.Name,
				Type:        resType,
				Path:        doc.Path,
				Description: doc.Description,
			},
			Origin:      origin,
			Revision:    doc.Revision,
			Fingerprint: fp,
			Size:        doc.Size,
		},
		ID:        doc.ID,
		PendingID: doc.PendingID,
		ServiceID: doc.ServiceID,
		Username:  doc.Username,
		Timestamp: doc.Timestamp,
	}
	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}
	return res, nil
}
