// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/description"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"
)

const defaultControllerPermission = description.LoginAccess

// ControllerUser represents a user access to an controller
// controller user always represents a single user for a single controller.
// There should be no more than one ControllerUser per controller.
type ControllerUser struct {
	st                   *State
	doc                  controllerUserDoc
	controllerPermission *permission
}

// NewControllerUser returns a ControllerUser with permissions.
func NewControllerUser(st *State, doc controllerUserDoc) (*ControllerUser, error) {
	mu := &ControllerUser{
		st:  st,
		doc: doc,
	}
	if err := mu.refreshPermission(); err != nil {
		return nil, errors.Trace(err)
	}
	return mu, nil
}

type controllerUserDoc struct {
	ID             string    `bson:"_id"`
	ControllerUUID string    `bson:"controller-uuid"`
	UserName       string    `bson:"user"`
	DisplayName    string    `bson:"displayname"`
	CreatedBy      string    `bson:"createdby"`
	DateCreated    time.Time `bson:"datecreated"`
}

// Access returns the access level of this controller user.
func (e *ControllerUser) Access() description.Access {
	return e.controllerPermission.access()
}

// ID returns the ID of the controller user.
func (e *ControllerUser) ID() string {
	return e.doc.ID
}

// UserTag returns the tag for the controller user.
func (e *ControllerUser) UserTag() names.UserTag {
	return names.NewUserTag(e.doc.UserName)
}

// UserName returns the user name of the controller user.
func (e *ControllerUser) UserName() string {
	return e.doc.UserName
}

// DisplayName returns the display name of the controller user.
func (e *ControllerUser) DisplayName() string {
	return e.doc.DisplayName
}

// CreatedBy returns the user who created the controller user.
func (e *ControllerUser) CreatedBy() string {
	return e.doc.CreatedBy
}

// DateCreated returns the date the controller user was created in UTC.
func (e *ControllerUser) DateCreated() time.Time {
	return e.doc.DateCreated.UTC()
}

// refreshPermission reloads the permission for this controller user from persistence.
func (e *ControllerUser) refreshPermission() error {
	perm, err := e.st.userPermission(controllerGlobalKey, e.globalKey())
	if err != nil {
		return errors.Annotate(err, "updating permission")
	}
	e.controllerPermission = perm
	return nil
}

// IsGreaterAccess returns true if provided access is higher than
// the current one.
func (e *ControllerUser) IsGreaterAccess(a description.Access) bool {
	return e.controllerPermission.isGreaterAccess(a)
}

// SetAccess changes the user's access permissions on the controller.
func (e *ControllerUser) SetAccess(access description.Access) error {
	if err := access.Validate(); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(controllerGlobalKey, e.globalKey(), access)
	err := e.st.runTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	if err != nil {
		return errors.Trace(err)
	}
	return e.refreshPermission()
}

func (e *ControllerUser) globalKey() string {
	// TODO(perrito666) this asumes out of band knowledge of how controllerUserID is crafted
	username := strings.ToLower(e.UserName())
	return userWithGlobalKey(username)
}

// ControllerUser returns the controller user.
func (st *State) ControllerUser(user names.UserTag) (*ControllerUser, error) {
	controllerUser := &ControllerUser{st: st}
	controllerUsers, closer := st.getCollection(controllerUsersC)
	defer closer()

	username := strings.ToLower(user.Canonical())
	err := controllerUsers.FindId(username).One(&controllerUser.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("controller user %q", user.Canonical())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	controllerUser.doc.DateCreated = controllerUser.doc.DateCreated.UTC()
	if err := controllerUser.refreshPermission(); err != nil {
		return nil, errors.Trace(err)
	}
	return controllerUser, nil
}

// controllerUserID returns the document id of the controller user
func controllerUserID(user names.UserTag) string {
	username := user.Canonical()
	return strings.ToLower(username)
}

func createControllerUserOps(controllerUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access description.Access) []txn.Op {
	creatorname := createdBy.Canonical()
	doc := &controllerUserDoc{
		ID:             controllerUserID(user),
		ControllerUUID: controllerUUID,
		UserName:       user.Canonical(),
		DisplayName:    displayName,
		CreatedBy:      creatorname,
		DateCreated:    dateCreated,
	}
	ops := []txn.Op{
		createPermissionOp(controllerGlobalKey, userWithGlobalKey(controllerUserID(user)), access),
		{
			C:      controllerUsersC,
			Id:     controllerUserID(user),
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	return ops
}

// RemoveControllerUser removes a user from the database.
func (st *State) RemoveControllerUser(user names.UserTag) error {
	ops := []txn.Op{
		removePermissionOp(controllerGlobalKey, userWithGlobalKey(controllerUserID(user))),
		{
			C:      controllerUsersC,
			Id:     controllerUserID(user),
			Assert: txn.DocExists,
			Remove: true,
		}}

	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("controller user %q does not exist", user.Canonical()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
