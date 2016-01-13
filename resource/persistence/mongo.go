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
func (p Persistence) resourceID(id, serviceID string) string {
	return fmt.Sprintf("resource#%s#%s", serviceID, id)
}

// stagedID converts an external resource ID into an internal staged one.
func (p Persistence) stagedID(id, serviceID string) string {
	return p.resourceID(id, serviceID) + stagedIDSuffix
}

func (p Persistence) newStagedResourceOps(id, serviceID string, res resource.Resource) []txn.Op {
	doc := p.newStagedDoc(id, serviceID, res)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func (p Persistence) newEnsureStagedSameOps(id, serviceID string, res resource.Resource) []txn.Op {
	doc := p.newStagedDoc(id, serviceID, res)

	// Other than cause the txn to abort, we don't do anything here.
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: doc, // TODO(ericsnow) Is this okay?
	}}
}

func (p Persistence) newRemoveStagedOps(id, serviceID string) []txn.Op {
	fullID := p.stagedID(id, serviceID)

	// We don't assert that it exists. We want "missing" to be a noop.
	return []txn.Op{{
		C:      resourcesC,
		Id:     fullID,
		Remove: true,
	}}
}

func (p Persistence) newInsertResourceOps(id, serviceID string, res resource.Resource) []txn.Op {
	doc := p.newResourceDoc(id, serviceID, res)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func (p Persistence) newUpdateResourceOps(id, serviceID string, res resource.Resource) []txn.Op {
	doc := p.newResourceDoc(id, serviceID, res)

	// TODO(ericsnow) Using "update" doesn't work right...
	return append([]txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}, p.newInsertResourceOps(id, serviceID, res)...)
}

// newResourceDoc generates a doc that represents the given resource.
func (p Persistence) newResourceDoc(id, serviceID string, res resource.Resource) *resourceDoc {
	fullID := p.resourceID(id, serviceID)
	return resource2doc(fullID, serviceID, res)
}

// newStagedDoc generates a staging doc that represents the given resource.
func (p Persistence) newStagedDoc(id, serviceID string, res resource.Resource) *resourceDoc {
	stagedID := p.stagedID(id, serviceID)
	return resource2doc(stagedID, serviceID, res)
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

// resource2doc converts the resource into a DB doc.
func resource2doc(id, serviceID string, res resource.Resource) *resourceDoc {
	// TODO(ericsnow) We may need to limit the resolution of timestamps
	// in order to avoid some conversion problems from Mongo.
	return &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

	fp, err := charmresource.NewFingerprint(doc.Fingerprint)
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
