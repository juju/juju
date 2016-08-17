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

// setAccess changes the user's access permissions on the controller.
func (st *State) setControllerAccess(access description.Access, userGlobalKey string) error {
	if err := description.ValidateControllerAccess(access); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(controllerKey(st.ControllerUUID()), userGlobalKey, access)

	err := st.runTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// controllerUser a model userAccessDoc.
func (st *State) controllerUser(user names.UserTag) (userAccessDoc, error) {
	controllerUser := userAccessDoc{}
	controllerUsers, closer := st.getCollection(controllerUsersC)
	defer closer()

	username := strings.ToLower(user.Canonical())
	err := controllerUsers.FindId(username).One(&controllerUser)
	if err == mgo.ErrNotFound {
		return userAccessDoc{}, errors.NotFoundf("controller user %q", user.Canonical())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	controllerUser.DateCreated = controllerUser.DateCreated.UTC()
	return controllerUser, nil
}

func createControllerUserOps(controllerUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access description.Access) []txn.Op {
	creatorname := createdBy.Canonical()
	doc := &userAccessDoc{
		ID:          userAccessID(user),
		ObjectUUID:  controllerUUID,
		UserName:    user.Canonical(),
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

// RemoveControllerUser removes a user from the database.
func (st *State) removeControllerUser(user names.UserTag) error {
	ops := []txn.Op{
		removePermissionOp(controllerKey(st.ControllerUUID()), userGlobalKey(userAccessID(user))),
		{
			C:      controllerUsersC,
			Id:     userAccessID(user),
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
