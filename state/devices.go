// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// DeviceType defines a device type.
type DeviceType string

// DeviceConstraints contains the user-specified constraints for allocating
// device instances for an application unit.
type DeviceConstraints struct {

	// Type is the device type or device-class.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`

	// Attributes is a collection of key value pairs device related (node affinity labels/tags etc.).
	Attributes map[string]string `bson:"attributes"`
}

// deviceConstraintsDoc contains device constraints for an entity.
type deviceConstraintsDoc struct {
	DocID       string                       `bson:"_id"`
	ModelUUID   string                       `bson:"model-uuid"`
	Constraints map[string]DeviceConstraints `bson:"constraints"`
}

func createDeviceConstraintsOp(key string, cons map[string]DeviceConstraints) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: &deviceConstraintsDoc{
			Constraints: cons,
		},
	}
}

func replaceDeviceConstraintsOp(key string, cons map[string]DeviceConstraints) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"constraints", cons}}}},
	}
}

func removeDeviceConstraintsOp(key string) txn.Op {
	return txn.Op{
		C:      deviceConstraintsC,
		Id:     key,
		Remove: true,
	}
}
func readDeviceConstraints(mb modelBackend, key string) (map[string]DeviceConstraints, error) {
	coll, closer := mb.db().GetCollection(deviceConstraintsC)
	defer closer()

	var doc deviceConstraintsDoc
	err := coll.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("device constraints for %q", key)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get device constraints for %q", key)
	}
	return doc.Constraints, nil
}
