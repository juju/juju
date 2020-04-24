// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/permission"
)

const defaultControllerPermission = permission.LoginAccess

// setAccess changes the user's access permissions on the controller.
func (st *State) setControllerAccess(access permission.Access, userGlobalKey string) error {
	if err := permission.ValidateControllerAccess(access); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(controllerKey(st.ControllerUUID()), userGlobalKey, access)

	err := st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// controllerUser a model userAccessDoc.
func (st *State) controllerUser(user names.UserTag) (userAccessDoc, error) {
	controllerUser := userAccessDoc{}
	controllerUsers, closer := st.db().GetCollection(controllerUsersC)
	defer closer()

	username := strings.ToLower(user.Id())
	err := controllerUsers.FindId(username).One(&controllerUser)
	if err == mgo.ErrNotFound {
		return userAccessDoc{}, errors.NotFoundf("controller user %q", user.Id())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	controllerUser.DateCreated = controllerUser.DateCreated.UTC()
	return controllerUser, nil
}

func createControllerUserOps(controllerUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access permission.Access) []txn.Op {
	creatorname := createdBy.Id()
	doc := &userAccessDoc{
		ID:          userAccessID(user),
		ObjectUUID:  controllerUUID,
		UserName:    user.Id(),
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: dateCreated,
	}
	ops := []txn.Op{
		createPermissionOp(controllerKey(controllerUUID), userGlobalKey(userAccessID(user)), access),
		{
			C:      controllerUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	return ops
}

func removeControllerUserOps(controllerUUID string, user names.UserTag) []txn.Op {
	return []txn.Op{
		removePermissionOp(controllerKey(controllerUUID), userGlobalKey(userAccessID(user))),
		{
			C:      controllerUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocExists,
			Remove: true,
		}}

}

// RemoveControllerUser removes a user from the database.
func (st *State) removeControllerUser(user names.UserTag) error {
	ops := removeControllerUserOps(st.ControllerUUID(), user)
	err := st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("controller user %q does not exist", user.Id()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
