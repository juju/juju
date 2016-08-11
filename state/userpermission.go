// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/description"
)

// permission represents the permission a user has
// on a given scope.
type permission struct {
	doc permissionDoc
}

type permissionDoc struct {
	ID string `bson:"_id"`
	// ObjectGlobalKey holds the id for the object of the permission.
	// ie. a model globalKey or a controller globalKey.
	ObjectGlobalKey string `bson:"object-global-key"`
	// SubjectGlobalKey holds the id for the user/group that is given permission.
	SubjectGlobalKey string `bson:"subject-global-key"`
	// Access is the permission level.
	Access string `bson:"access"`
}

func stringToAccess(a string) description.Access {
	return description.Access(a)
}

func accessToString(a description.Access) string {
	return string(a)
}

// userPermission returns a Permission for the given Subject and User.
func (st *State) userPermission(objectGlobalKey, subjectGlobalKey string) (*permission, error) {
	userPermission := &permission{}
	permissions, closer := st.getCollection(permissionsC)
	defer closer()

	id := permissionID(objectGlobalKey, subjectGlobalKey)
	err := permissions.FindId(id).One(&userPermission.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("user permissions for user %q", id)
	}
	return userPermission, nil
}

// controllerUserPermission returns a Permission for the given Subject and User.
func (st *State) controllerUserPermission(objectGlobalKey, subjectGlobalKey string) (*permission, error) {
	userPermission := &permission{}

	permissions, closer := st.getCollection(permissionsC)
	defer closer()

	id := permissionID(objectGlobalKey, subjectGlobalKey)
	err := permissions.FindId(id).One(&userPermission.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("user permissions for user %q", id)
	}
	return userPermission, nil
}

// isReadOnly returns whether or not the user has write access or only
// read access to the model.
func (p *permission) isReadOnly() bool {
	return stringToAccess(p.doc.Access) == description.UndefinedAccess || stringToAccess(p.doc.Access) == description.ReadAccess
}

// isAdmin is a convenience method that
// returns whether or not the user has description.AdminAccess.
func (p *permission) isAdmin() bool {
	return stringToAccess(p.doc.Access) == description.AdminAccess
}

// isReadWrite is a convenience method that
// returns whether or not the user has description.WriteAccess.
func (p *permission) isReadWrite() bool {
	return stringToAccess(p.doc.Access) == description.WriteAccess
}

func (p *permission) access() description.Access {
	return stringToAccess(p.doc.Access)
}

func permissionID(objectGlobalKey, subjectGlobalKey string) string {
	// example: e#:deadbeef#us#jim
	// e: object global key
	// deadbeef: object uuid
	// us#jim: subject global key
	// the first element (e in this example) is the global key for the object
	// (model in this example)
	// the second, is the : prefixed model uuid
	// the third, in this example is a user with name jim, hence the globalKey
	// ( a user global key) being us#jim.
	// another example, now with controller and user maria:
	// c#:deadbeef#us#maria
	// c: object global key, in this case controller.
	// :deadbeef controller uuid
	// us#maria: its the user global key for maria.
	// if this where for model, it would be e#us#maria
	return fmt.Sprintf("%s#%s", objectGlobalKey, subjectGlobalKey)
}

func updatePermissionOp(objectGlobalKey, subjectGlobalKey string, access description.Access) txn.Op {
	return txn.Op{
		C:      permissionsC,
		Id:     permissionID(objectGlobalKey, subjectGlobalKey),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"access", accessToString(access)}}}},
	}
}

func removePermissionOp(objectGlobalKey, subjectGlobalKey string) txn.Op {
	return txn.Op{
		C:      permissionsC,
		Id:     permissionID(objectGlobalKey, subjectGlobalKey),
		Assert: txn.DocExists,
		Remove: true,
	}

}
func createPermissionOp(objectGlobalKey, subjectGlobalKey string, access description.Access) txn.Op {
	doc := &permissionDoc{
		ID:               permissionID(objectGlobalKey, subjectGlobalKey),
		SubjectGlobalKey: subjectGlobalKey,
		ObjectGlobalKey:  objectGlobalKey,
		Access:           accessToString(access),
	}
	return txn.Op{
		C:      permissionsC,
		Id:     permissionID(objectGlobalKey, subjectGlobalKey),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}
