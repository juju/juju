// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// linkLayerDevicesRefsDoc associates each known link-layer network device on a
// machine with the number of its "children" devices (if any) on the same
// machine.
type linkLayerDevicesRefsDoc struct {
	// DocID is the (parent) device DocID (global key prefixed by ModelUUID).
	DocID string `bson:"_id"`

	// ModelUUID is the UUID of the model the device belongs to.
	ModelUUID string `bson:"model-uuid"`

	// NumChildren is number of devices which refer to this device as their
	// parent.
	NumChildren int `bson:"num-children"`
}

// insertLinkLayerDevicesRefsOp returns an operation to insert a new
// linkLayerDevicesRefsDoc for the given modelUUID and linkLayerDeviceDocID,
// with NumChildren=0.
func insertLinkLayerDevicesRefsOp(modelUUID, linkLayerDeviceDocID string) txn.Op {
	refsDoc := &linkLayerDevicesRefsDoc{
		DocID:       linkLayerDeviceDocID,
		ModelUUID:   modelUUID,
		NumChildren: 0,
	}
	return txn.Op{
		C:      linkLayerDevicesRefsC,
		Id:     linkLayerDeviceDocID,
		Assert: txn.DocMissing,
		Insert: refsDoc,
	}
}

// removeLinkLayerDevicesRefsOp returns an operation to remove the
// linkLayerDevicesRefsDoc for the given linkLayerDeviceDocID, asserting the
// document has NumChildren == 0.
func removeLinkLayerDevicesRefsOp(linkLayerDeviceDocID string) txn.Op {
	hasNoChildren := bson.D{{"num-children", 0}}
	return txn.Op{
		C:      linkLayerDevicesRefsC,
		Id:     linkLayerDeviceDocID,
		Assert: hasNoChildren,
		Remove: true,
	}
}

// getParentDeviceNumChildrenRefs returns the NumChildren value for the given
// linkLayerDeviceDocID. If the linkLayerDevicesRefsDoc is missing, no error and
// -1 children are returned.
func getParentDeviceNumChildrenRefs(st *State, linkLayerDeviceDocID string) (int, error) {
	devicesRefs, closer := st.getCollection(linkLayerDevicesRefsC)
	defer closer()

	var doc linkLayerDevicesRefsDoc
	err := devicesRefs.FindId(linkLayerDeviceDocID).One(&doc)
	if err == mgo.ErrNotFound {
		return -1, nil
	} else if err != nil {
		return 0, errors.Trace(err)
	}
	return doc.NumChildren, nil
}

// incrementDeviceNumChildrenOp returns an operation that increments the
// NumChildren value of the linkLayerDevicesRefsDoc matching the given
// linkLayerDeviceDocID, and asserting the document has NumChildren >= 0.
func incrementDeviceNumChildrenOp(linkLayerDeviceDocID string) txn.Op {
	hasZeroOrMoreChildren := bson.D{{"num-children", bson.D{{"$gte", 0}}}}
	return txn.Op{
		C:      linkLayerDevicesRefsC,
		Id:     linkLayerDeviceDocID,
		Assert: hasZeroOrMoreChildren,
		Update: bson.D{{"$inc", bson.D{{"num-children", 1}}}},
	}
}

// decrementDeviceNumChildrenOp returns an operation that decrements the
// NumChildren value of the linkLayerDevicesRefsDoc matching the given
// linkLayerDeviceDocID, and asserting the document has NumChildren >= 1.
func decrementDeviceNumChildrenOp(linkLayerDeviceDocID string) txn.Op {
	hasAtLeastOneMoreChild := bson.D{{"num-children", bson.D{{"$gte", 1}}}}
	return txn.Op{
		C:      linkLayerDevicesRefsC,
		Id:     linkLayerDeviceDocID,
		Assert: hasAtLeastOneMoreChild,
		Update: bson.D{{"$inc", bson.D{{"num-children", -1}}}},
	}
}
