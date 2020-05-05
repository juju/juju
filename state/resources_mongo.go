// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/kr/pretty"
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

func applicationResourceID(id string) string {
	return resourceID(id, "", "")
}

func pendingResourceID(id, pendingID string) string {
	return resourceID(id, "pending", pendingID)
}

func charmStoreResourceID(id string) string {
	return applicationResourceID(id) + resourcesCharmstoreIDSuffix
}

func unitResourceID(id, unitID string) string {
	return resourceID(id, "unit", unitID)
}

// stagedResourceID converts an external resource ID into an internal
// staged one.
func stagedResourceID(id string) string {
	return applicationResourceID(id) + resourcesStagedIDSuffix
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
	id            string
	applicationID string
	lastPolled    time.Time
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

func resourceDocToUpdateOp(doc *resourceDoc) bson.M {
	// Note (jam 2018-07-12): What are we actually allowed to update?
	// The old code was trying to delete the doc and replace it entirely, so for now we'll just set everything except
	// the doc's own id, which is clearly not allowed to change.
	return bson.M{"$set": bson.M{
		"resource-id":                doc.ID,
		"pending-id":                 doc.PendingID,
		"application-id":             doc.ApplicationID,
		"unit-id":                    doc.UnitID,
		"name":                       doc.Name,
		"type":                       doc.Type,
		"path":                       doc.Path,
		"description":                doc.Description,
		"origin":                     doc.Origin,
		"revision":                   doc.Revision,
		"fingerprint":                doc.Fingerprint,
		"size":                       doc.Size,
		"username":                   doc.Username,
		"timestamp-when-added":       doc.Timestamp,
		"storage-path":               doc.StoragePath,
		"download-progress":          doc.DownloadProgress,
		"timestamp-when-last-polled": doc.LastPolled,
	}}
}

func newUpdateResourceOps(stored storedResource) []txn.Op {
	doc := newResourceDoc(stored)

	logger.Tracef("updating resource %s to %# v", stored.ID, pretty.Formatter(doc))
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Update: resourceDocToUpdateOp(doc),
	}}
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

	logger.Tracef("updating charm store resource %s to %# v", res.id, pretty.Formatter(doc))
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Update: resourceDocToUpdateOp(doc),
	}}
}

func newInsertUnitResourceOps(unitID string, stored storedResource, progress *int64) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)
	doc.DownloadProgress = progress

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateUnitResourceOps(unitID string, stored storedResource, progress *int64) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)
	doc.DownloadProgress = progress

	logger.Tracef("updating unit resource %s to %# v", unitID, pretty.Formatter(doc))
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists, // feels like we need more
		Update: resourceDocToUpdateOp(doc),
	}}
}

func newRemoveResourcesOps(docs []resourceDoc) []txn.Op {
	// The likelihood of a race is small and the consequences are minor,
	// so we don't worry about the corner case of missing a doc here.
	var ops []txn.Op
	for _, doc := range docs {
		// We do not bother to assert txn.DocExists since it will be
		// gone either way, which is the result we're after.
		ops = append(ops, txn.Op{
			C:      resourcesC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops
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

	// TODO(perrito666) 2016-05-02 lp:1558657
	csRes := charmStoreResource{
		Resource:      newRes.Resource.Resource,
		id:            newRes.ID,
		applicationID: newRes.ApplicationID,
		// Truncate the time to remove monotonic time for Go 1.9+
		// to make it easier for tests to compare the time.
		lastPolled: time.Now().Truncate(1).UTC(),
	}

	if exists {
		ops = append(ops, newUpdateResourceOps(newRes)...)
		return append(ops, newUpdateCharmStoreResourceOps(csRes)...)
	} else {
		ops = append(ops, newInsertResourceOps(newRes)...)
		return append(ops, newInsertCharmStoreResourceOps(csRes)...)
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
	fullID := applicationResourceID(stored.ID)
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

// resources returns the resource docs for the given application.
func (p ResourcePersistence) resources(applicationID string) ([]resourceDoc, error) {
	logger.Tracef("querying db for resources for %q", applicationID)
	var docs []resourceDoc
	query := bson.D{{"application-id", applicationID}}
	if err := p.base.All(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("found %d resources", len(docs))
	return docs, nil
}

func (p ResourcePersistence) unitResources(unitID string) ([]resourceDoc, error) {
	var docs []resourceDoc
	query := bson.D{{"unit-id", unitID}}
	if err := p.base.All(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// getOne returns the resource that matches the provided model ID.
func (p ResourcePersistence) getOne(resID string) (resourceDoc, error) {
	logger.Tracef("querying db for resource %q", resID)
	id := applicationResourceID(resID)
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
	ID        string `bson:"resource-id"`
	PendingID string `bson:"pending-id"`

	ApplicationID string `bson:"application-id"`
	UnitID        string `bson:"unit-id"`

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

	DownloadProgress *int64 `bson:"download-progress,omitempty"`

	LastPolled time.Time `bson:"timestamp-when-last-polled"`
}

func charmStoreResource2Doc(id string, res charmStoreResource) *resourceDoc {
	stored := storedResource{
		Resource: resource.Resource{
			Resource:      res.Resource,
			ID:            res.id,
			ApplicationID: res.applicationID,
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

		ApplicationID: res.ApplicationID,

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
		ID:            doc.ID,
		PendingID:     doc.PendingID,
		ApplicationID: doc.ApplicationID,
		Username:      doc.Username,
		Timestamp:     doc.Timestamp,
	}
	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}
	return res, nil
}
