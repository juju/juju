// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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
	Access Access `bson:"access"`
}

// userPermission returns a Permission for the given Subject and User.
func (st *State) userPermission(objectKey, subjectKey string) (*permission, error) {
	userPermission := &permission{}
	permissions, closer := st.getCollection(permissionsC)
	defer closer()

	err := permissions.FindId(permissionID(objectKey, subjectKey)).One(&userPermission.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("user permissions for user %q", subjectKey)
	}
	return userPermission, nil

}

// isReadOnly returns whether or not the user has write access or only
// read access to the model.
func (p *permission) isReadOnly() bool {
	return p.doc.Access == UndefinedAccess || p.doc.Access == ReadAccess
}

// isAdmin is a convenience method that
// returns whether or not the user has AdminAccess.
func (p *permission) isAdmin() bool {
	return p.doc.Access == AdminAccess
}

// isReadWrite is a convenience method that
// returns whether or not the user has WriteAccess.
func (p *permission) isReadWrite() bool {
	return p.doc.Access == WriteAccess
}

func (p *permission) access() Access {
	return p.doc.Access
}

func (p *permission) isGreaterAccess(a Access) bool {
	switch p.doc.Access {
	case UndefinedAccess:
		return a == ReadAccess || a == WriteAccess || a == AdminAccess
	case ReadAccess:
		return a == WriteAccess || a == AdminAccess
	case WriteAccess:
		return a == AdminAccess
	}
	return false
}

func permissionID(objectKey, subjectKey string) string {
	// example: e#mo#jim
	// e: model global key (its always e).
	// mo: model user key prefix.
	// jim: an arbitrary username.
	return fmt.Sprintf("%s#%s", objectKey, subjectKey)
}

func updatePermissionOp(objectGlobalKey, subjectGlobalKey string, access Access) txn.Op {
	return txn.Op{
		C:      permissionsC,
		Id:     permissionID(objectGlobalKey, subjectGlobalKey),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"access", access}}}},
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
func createPermissionOp(objectGlobalKey, subjectGlobalKey string, access Access) txn.Op {
	doc := &permissionDoc{
		ID:               permissionID(objectGlobalKey, subjectGlobalKey),
		SubjectGlobalKey: subjectGlobalKey,
		ObjectGlobalKey:  objectGlobalKey,
		Access:           access,
	}
	return txn.Op{
		C:      permissionsC,
		Id:     permissionID(objectGlobalKey, subjectGlobalKey),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// Access represents the level of access granted to a user on a model.
type Access string

const (
	// UndefinedAccess is not a valid access type. It is the value
	// unmarshaled when access is not defined by the document at all.
	UndefinedAccess Access = ""

	// ReadAccess allows a user to read information about a model, without
	// being able to make any changes.
	ReadAccess Access = "read"

	// WriteAccess allows a user to make changes to a model.
	WriteAccess Access = "write"

	// AdminAccess allows a user full control over the model.
	AdminAccess Access = "admin"
)
