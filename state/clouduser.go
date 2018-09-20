// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/permission"
)

// CreateCloudAccess creates a new access permission for a user on a cloud.
func (st *State) CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	if err := permission.ValidateCloudAccess(access); err != nil {
		return errors.Trace(err)
	}

	// Local users must exist.
	if user.IsLocal() {
		_, err := st.User(user)
		if err != nil {
			if errors.IsNotFound(err) {
				return errors.Annotatef(err, "user %q does not exist locally", user.Name())
			}
			return errors.Trace(err)
		}
	}

	op := createPermissionOp(cloudGlobalKey(cloud), userGlobalKey(userAccessID(user)), access)

	err := st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("permission for user %q for cloud %q", user.Id(), cloud)
	}
	return errors.Trace(err)
}

// GetCloudAccess gets the access permission for the specified user on a cloud.
func (st *State) GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error) {
	perm, err := st.userPermission(cloudGlobalKey(cloud), userGlobalKey(userAccessID(user)))
	if err != nil {
		return "", errors.Trace(err)
	}
	return perm.access(), nil
}

// GetCloudUsers gets the access permissions on a cloud.
func (st *State) GetCloudUsers(cloud string) (map[string]permission.Access, error) {
	perms, err := st.usersPermissions(cloudGlobalKey(cloud))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]permission.Access)
	for _, p := range perms {
		result[userIDFromGlobalKey(p.doc.SubjectGlobalKey)] = p.access()
	}
	return result, nil
}

// UpdateCloudAccess changes the user's access permissions on a cloud.
func (st *State) UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	if err := permission.ValidateCloudAccess(access); err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(int) ([]txn.Op, error) {
		_, err := st.GetCloudAccess(cloud, user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := []txn.Op{updatePermissionOp(cloudGlobalKey(cloud), userGlobalKey(userAccessID(user)), access)}
		return ops, nil
	}

	err := st.db().Run(buildTxn)
	return errors.Trace(err)
}

// RemoveCloudAccess removes the access permission for a user on a cloud.
func (st *State) RemoveCloudAccess(cloud string, user names.UserTag) error {
	buildTxn := func(int) ([]txn.Op, error) {
		_, err := st.GetCloudAccess(cloud, user)
		if err != nil {
			return nil, err
		}
		ops := []txn.Op{removePermissionOp(cloudGlobalKey(cloud), userGlobalKey(userAccessID(user)))}
		return ops, nil
	}

	err := st.db().Run(buildTxn)
	return errors.Trace(err)
}
