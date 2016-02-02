// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

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

	stagedIDSuffix = "#staged"
)

// resourceID converts an external resource ID into an internal one.
func resourceID(id, ownerID string) string {
	return fmt.Sprintf("resource#%s#%s", ownerID, id)
}

// stagedID converts an external resource ID into an internal staged one.
func stagedID(id, serviceID string) string {
	return resourceID(id, serviceID) + stagedIDSuffix
}

func newStagedResourceOps(args resource.ModelResource) []txn.Op {
	doc := newStagedDoc(args)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newEnsureStagedSameOps(args resource.ModelResource) []txn.Op {
	doc := newStagedDoc(args)

	// Other than cause the txn to abort, we don't do anything here.
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: doc, // TODO(ericsnow) Is this okay?
	}}
}

func newRemoveStagedOps(id, serviceID string) []txn.Op {
	fullID := stagedID(id, serviceID)

	// We don't assert that it exists. We want "missing" to be a noop.
	return []txn.Op{{
		C:      resourcesC,
		Id:     fullID,
		Remove: true,
	}}
}

func newInsertUnitResourceOps(unitID string, args resource.ModelResource) []txn.Op {
	doc := newUnitResourceDoc(unitID, args)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newInsertResourceOps(args resource.ModelResource) []txn.Op {
	doc := newResourceDoc(args)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateUnitResourceOps(unitID string, args resource.ModelResource) []txn.Op {
	doc := newUnitResourceDoc(unitID, args)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, newInsertUnitResourceOps(unitID, args)...)
}

func newUpdateResourceOps(args resource.ModelResource) []txn.Op {
	doc := newResourceDoc(args)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, newInsertResourceOps(args)...)
}

// newUnitResourceDoc generates a doc that represents the given resource.
func newUnitResourceDoc(unitID string, args resource.ModelResource) *resourceDoc {
	fullID := resourceID(args.ID, unitID)
	return unitResource2Doc(fullID, unitID, args)
}

// newResourceDoc generates a doc that represents the given resource.
func newResourceDoc(args resource.ModelResource) *resourceDoc {
	fullID := resourceID(args.ID, args.ServiceID)
	return resource2doc(fullID, args)
}

// newStagedDoc generates a staging doc that represents the given resource.
func newStagedDoc(args resource.ModelResource) *resourceDoc {
	stagedID := stagedID(args.ID, args.ServiceID)
	return resource2doc(stagedID, args)
}

// resources returns the resource docs for the given service.
func (p Persistence) resources(serviceID string) ([]resourceDoc, error) {
	logger.Tracef("querying db for resources for %q", serviceID)
	var docs []resourceDoc
	query := bson.D{{"service-id", serviceID}}
	if err := p.base.All(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("found %d resources", len(docs))
	return docs, nil
}

// resourceDoc is the top-level document for resources.
type resourceDoc struct {
	DocID     string `bson:"_id"`
	EnvUUID   string `bson:"env-uuid"`
	ServiceID string `bson:"service-id"`
	UnitID    string `bson:"unit-id"`
	PendingID string `bson:"pending-id"`

	Name    string `bson:"name"`
	Type    string `bson:"type"`
	Path    string `bson:"path"`
	Comment string `bson:"comment"`

	Origin      string `bson:"origin"`
	Revision    int    `bson:"revision"`
	Fingerprint []byte `bson:"fingerprint"`
	Size        int64  `bson:"size"`

	Username  string    `bson:"username"`
	Timestamp time.Time `bson:"timestamp-when-added"`
}

func unitResource2Doc(id, unitID string, args resource.ModelResource) *resourceDoc {
	doc := resource2doc(id, args)
	doc.UnitID = unitID
	return doc
}

// resource2doc converts the resource into a DB doc.
func resource2doc(id string, args resource.ModelResource) *resourceDoc {
	res := args.Resource
	// TODO(ericsnow) We may need to limit the resolution of timestamps
	// in order to avoid some conversion problems from Mongo.
	return &resourceDoc{
		DocID:     id,
		ServiceID: args.ServiceID,
		PendingID: args.PendingID,

		Name:    res.Name,
		Type:    res.Type.String(),
		Path:    res.Path,
		Comment: res.Comment,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,

		Username:  res.Username,
		Timestamp: res.Timestamp,
	}
}

// TODO(ericsnow) Return resource.ModelResource from doc2resource()?

// doc2resource returns the resource.Resource represented by the doc.
func doc2resource(doc resourceDoc) (resource.Resource, error) {
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
				Name:    doc.Name,
				Type:    resType,
				Path:    doc.Path,
				Comment: doc.Comment,
			},
			Origin:      origin,
			Revision:    doc.Revision,
			Fingerprint: fp,
			Size:        doc.Size,
		},
		Username:  doc.Username,
		Timestamp: doc.Timestamp,
	}
	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}
	return res, nil
}
