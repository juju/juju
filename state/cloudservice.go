// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// CloudServicer represents the state of a CAAS service.
type CloudServicer interface {
	// ProviderId returns the id assigned to the service
	// by the cloud.
	ProviderId() string

	// Addresses returns the service addresses.
	Addresses() []network.Address
}

// CloudService is an implementation of CloudService.
type CloudService struct {
	st  *State
	doc cloudServiceDoc
}

type cloudServiceDoc struct {
	// DocID holds cloud service document key.
	DocID string `bson:"_id"`

	ProviderId string    `bson:"provider-id"`
	Addresses  []address `bson:"addresses"`
}

func newCloudService(st *State, doc *cloudServiceDoc) *CloudService {
	svc := &CloudService{
		st:  st,
		doc: *doc,
	}
	return svc
}

// Id implements CloudService.
func (c *CloudService) Id() string {
	return c.doc.DocID
}

// ProviderId implements CloudService.
func (c *CloudService) ProviderId() string {
	return c.doc.ProviderId
}

// Addresses implements CloudService.
func (c *CloudService) Addresses() []network.Address {
	return networkAddresses(c.doc.Addresses)
}

func (c *CloudService) cloudServiceDoc() (*cloudServiceDoc, error) {
	coll, closer := c.st.db().GetCollection(cloudServicesC)
	defer closer()

	var doc cloudServiceDoc
	err := coll.FindId(c.Id()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("cloud service %v", c.Id())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// CloudService return the content of cloud service from the underlying state.
// It returns an error that satisfies errors.IsNotFound if the cloud service has been removed.
func (c *CloudService) CloudService() (*CloudService, error) {
	doc, err := c.cloudServiceDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if doc == nil {
		return nil, errors.NotFoundf("cloud service %v", c.Id())
	}
	c.doc = *doc
	return c, nil
}

// Refresh refreshes the content of cloud service from the underlying state.
// It returns an error that satisfies errors.IsNotFound if the cloud service has been removed.
func (c *CloudService) Refresh() error {
	_, err := c.CloudService()
	return errors.Trace(err)
}

func (c *CloudService) saveServiceOps(doc cloudServiceDoc) ([]txn.Op, error) {
	existing, err := c.cloudServiceDoc()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil || existing == nil {
		return []txn.Op{{
			C:      cloudServicesC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	return []txn.Op{{
		C:  cloudServicesC,
		Id: existing.DocID,
		Assert: bson.D{{"$or", []bson.D{
			{{"provider-id", doc.ProviderId}},
			{{"provider-id", bson.D{{"$exists", false}}}},
		}}},
		Update: bson.D{
			{"$set",
				bson.D{
					{"provider-id", doc.ProviderId},
					{"addresses", doc.Addresses},
				},
			},
		},
	}}, nil
}

func (a *Application) removeCloudServiceOps() []txn.Op {
	ops := []txn.Op{{
		C:      cloudServicesC,
		Id:     a.globalKey(),
		Remove: true,
	}}
	return ops
}
