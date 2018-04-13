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

// CloudService represents the state of a CAAS service.
type CloudService interface {
	// ProviderId returns the id assigned to the service
	// by the cloud.
	ProviderId() string

	// Addresses returns the service addresses.
	Addresses() []network.Address
}

// cloudService is an implementation of CloudService.
type cloudService struct {
	doc cloudServiceDoc
}

type cloudServiceDoc struct {
	// Id holds cloud service document key.
	// It is the global key of the application represented
	// by this service.
	Id string `bson:"_id"`

	ProviderId string    `bson:"provider-id"`
	Addresses  []address `bson:"addresses"`
}

// Id implements CloudService.
func (c *cloudService) Id() string {
	return c.doc.Id
}

// ProviderId implements CloudService.
func (c *cloudService) ProviderId() string {
	return c.doc.ProviderId
}

// Address implements CloudService.
func (c *cloudService) Addresses() []network.Address {
	return networkAddresses(c.doc.Addresses)
}

func (a *Application) cloudService() (*cloudServiceDoc, error) {
	coll, closer := a.st.db().GetCollection(cloudServicesC)
	defer closer()

	var doc cloudServiceDoc
	err := coll.FindId(a.globalKey()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("cloud service for application %v", a.Name())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (a *Application) saveServiceOps(doc cloudServiceDoc) ([]txn.Op, error) {
	existing, err := a.cloudService()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return []txn.Op{{
			C:      cloudServicesC,
			Id:     doc.Id,
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
		Id:     existing.Id,
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
