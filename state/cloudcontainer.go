// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// CloudContainer represents the state of a CAAS container, eg pod.
type CloudContainer interface {
	// ProviderId returns the id assigned to the container/pod
	// by the cloud.
	ProviderId() string

	// Address returns the container address.
	Address() string

	// Ports returns the open container ports.
	Ports() []string
}

// cloudContainer is an implementation of CloudContainer.
type cloudContainer struct {
	doc cloudContainerDoc
}

type cloudContainerDoc struct {
	// Id holds cloud container document key.
	// It is the global key of the unit represented
	// by this container.
	Id string `bson:"_id"`

	ProviderId string   `bson:"provider-id"`
	Address    string   `bson:"address"`
	Ports      []string `bson:"ports"`
}

// Id implements CloudContainer.
func (c *cloudContainer) Id() string {
	return c.doc.Id
}

// ProviderId implements CloudContainer.
func (c *cloudContainer) ProviderId() string {
	return c.doc.ProviderId
}

// Address implements CloudContainer.
func (c *cloudContainer) Address() string {
	return c.doc.Address
}

// Ports implements CloudContainer.
func (c *cloudContainer) Ports() []string {
	return c.doc.Ports
}

func (u *Unit) cloudContainer() (*cloudContainerDoc, error) {
	coll, closer := u.st.db().GetCollection(cloudContainersC)
	defer closer()

	var doc cloudContainerDoc
	err := coll.FindId(u.globalKey()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("cloud container for unit %v", u.Name())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (u *Unit) saveContainerOps(doc cloudContainerDoc) ([]txn.Op, error) {
	existing, err := u.cloudContainer()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		return []txn.Op{{
			C:      cloudContainersC,
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
		asserts = bson.D{{"$or", []bson.D{{providerValueAssert}, {{"$exists", false}}}}}
	}
	return []txn.Op{{
		C:      cloudContainersC,
		Id:     existing.Id,
		Assert: asserts,
		Update: bson.D{
			{"$set",
				bson.D{{"provider-id", doc.ProviderId},
					{"ports", doc.Ports},
					{"address", doc.Address}},
			},
		},
	}}, nil
}

func (u *Unit) removeCloudContainerOps() []txn.Op {
	ops := []txn.Op{{
		C:      cloudContainersC,
		Id:     u.globalKey(),
		Remove: true,
	}}
	return ops
}
