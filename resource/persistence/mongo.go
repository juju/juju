// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/resource"
)

const (
	resourcesC = "resources"
)

// TODO(ericsnow) Move the methods under their own type (resourcecollection?).

// resourceID converts an external resource ID into an internal one.
func (p Persistence) resourceID(id, serviceID string) string {
	return fmt.Sprintf("resource#%s#%s", serviceID, id)
}

// newResourceDoc generates a doc that represents the given resource.
func (p Persistence) newResourcDoc(id, serviceID string, res resource.Resource) *resourceDoc {
	id = p.resourceID(id, serviceID)

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

		Username:  res.Username,
		Timestamp: res.Timestamp,
	}
}

// resources returns the resource docs for the given service.
func (p Persistence) resources(serviceID string) ([]resourceDoc, error) {
	var docs []resourceDoc
	query := bson.D{{"service-id", serviceID}}
	if err := p.base.All(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
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

	Username  string    `bson:"username"`
	Timestamp time.Time `bson:"timestamp-when-added"`
}

// resource returns the resource.Resource represented by the doc.
func (doc resourceDoc) resource() (resource.Resource, error) {
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
		},
		Username:  doc.Username,
		Timestamp: doc.Timestamp,
	}
	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}
	return res, nil
}
