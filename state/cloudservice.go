// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/network"
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

func (c *CloudService) cloudService() (cloudServiceDoc, error) {
	coll, closer := c.st.db().GetCollection(cloudServicesC)
	defer closer()

	var doc cloudServiceDoc
	err := coll.FindId(c.Id()).One(&doc)
	if err == mgo.ErrNotFound {
		return cloudServiceDoc{}, errors.NotFoundf("cloud service %v", c.Id())
	}
	if err != nil {
		return cloudServiceDoc{}, errors.Trace(err)
	}
	return doc, nil
}

// Refresh refreshes the content of cloud service from the underlying state.
// It returns an error that satisfies errors.IsNotFound if the cloud service has been removed.
func (c *CloudService) Refresh() error {
	doc, err := c.cloudService()
	if err != nil {
		return errors.Trace(err)
	}
	c.doc = doc
	return nil
}

func (c *CloudService) saveServiceOps(doc cloudServiceDoc) ([]txn.Op, error) {
	existing, err := c.cloudService()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return []txn.Op{{
			C:      cloudServicesC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	var asserts bson.D
	providerValueAssert := bson.DocElem{"provider-id", existing.ProviderId}
	if existing.ProviderId != "" {
		asserts = bson.D{providerValueAssert}
	} else {
		asserts = bson.D{{"$or",
			[]bson.D{{providerValueAssert}, {{"provider-id", bson.D{{"$exists", false}}}}}}}
	}
	return []txn.Op{{
		C:      cloudServicesC,
		Id:     existing.DocID,
		Assert: asserts,
		Update: bson.D{
			{"$set",
				bson.D{{"provider-id", doc.ProviderId},
					{"addresses", doc.Addresses}},
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
