// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/permission"
)

// permission represents the permission a user has
// on a given scope.
type userPermission struct {
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

func stringToAccess(a string) permission.Access {
	return permission.Access(a)
}

func accessToString(a permission.Access) string {
	return string(a)
}

// userPermission returns a Permission for the given Subject and User.
func (st *State) userPermission(objectGlobalKey, subjectGlobalKey string) (*userPermission, error) {
	result := &userPermission{}
	permissions, closer := st.getCollection(permissionsC)
	defer closer()

	id := permissionID(objectGlobalKey, subjectGlobalKey)
	err := permissions.FindId(id).One(&result.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("user permissions for user %q", id)
	}
	return result, nil
}

// controllerUserPermission returns a Permission for the given Subject and User.
func (st *State) controllerUserPermission(objectGlobalKey, subjectGlobalKey string) (*userPermission, error) {
	result := &userPermission{}

	permissions, closer := st.getCollection(permissionsC)
	defer closer()

	id := permissionID(objectGlobalKey, subjectGlobalKey)
	err := permissions.FindId(id).One(&result.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("user permissions for user %q", id)
	}
	return result, nil
}

// isReadOnly returns whether or not the user has write access or only
// read access to the model.
func (p *userPermission) isReadOnly() bool {
	return stringToAccess(p.doc.Access) == permission.UndefinedAccess || stringToAccess(p.doc.Access) == permission.ReadAccess
}

// isAdmin is a convenience method that
// returns whether or not the user has permission.AdminAccess.
func (p *userPermission) isAdmin() bool {
	return stringToAccess(p.doc.Access) == permission.AdminAccess
}

// isReadWrite is a convenience method that
// returns whether or not the user has permission.WriteAccess.
func (p *userPermission) isReadWrite() bool {
	return stringToAccess(p.doc.Access) == permission.WriteAccess
}

func (p *userPermission) access() permission.Access {
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

func updatePermissionOp(objectGlobalKey, subjectGlobalKey string, access permission.Access) txn.Op {
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
func createPermissionOp(objectGlobalKey, subjectGlobalKey string, access permission.Access) txn.Op {
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
