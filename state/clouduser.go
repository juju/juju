// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
)

// cloudGlobalKey will return the key for a given cloud.
func cloudGlobalKey(cloudName string) string {
	return fmt.Sprintf("cloud#%s", cloudName)
}

// CreateCloudAccess creates a new access permission for a user on a cloud.
func (st *State) CreateCloudAccess(usr coreuser.User, cloud string, user names.UserTag, access permission.Access) error {
	if err := permission.ValidateCloudAccess(access); err != nil {
		return errors.Trace(err)
	}

	// Local users must exist.
	if !names.NewUserTag(usr.Name).IsLocal() {
		return errors.Errorf("user %q does not exist locally", usr.Name)
	}

	op := createPermissionOp(cloudGlobalKey(cloud), coreuser.UserGlobalKey(userAccessID(user)), access)

	err := st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("permission for user %q for cloud %q", user.Id(), cloud)
	}
	return errors.Trace(err)
}

// GetCloudAccess gets the access permission for the specified user on a cloud.
func (st *State) GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error) {
	perm, err := st.userPermission(cloudGlobalKey(cloud), coreuser.UserGlobalKey(userAccessID(user)))
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
		result[coreuser.UserIDFromGlobalKey(p.doc.SubjectGlobalKey)] = p.access()
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
		ops := []txn.Op{updatePermissionOp(cloudGlobalKey(cloud), coreuser.UserGlobalKey(userAccessID(user)), access)}
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
		ops := []txn.Op{removePermissionOp(cloudGlobalKey(cloud), coreuser.UserGlobalKey(userAccessID(user)))}
		return ops, nil
	}

	err := st.db().Run(buildTxn)
	return errors.Trace(err)
}

// CloudsForUser returns details including access level of clouds which can
// be seen by the specified user.
func (st *State) CloudsForUser(user names.UserTag) ([]cloud.CloudAccess, error) {
	cloudNames, err := st.cloudNamesForUser(user)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make([]cloud.CloudAccess, len(cloudNames))
	for i, name := range cloudNames {
		result[i] = cloud.CloudAccess{
			Name: name,
		}
	}
	if err := st.fillInCloudUserAccess(user, result); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// cloudNamesForUser returns the cloud names a user can see.
func (st *State) cloudNamesForUser(user names.UserTag) ([]string, error) {
	// Start by looking up cloud names that the user has access to, and then load only the records that are
	// included in that set
	permissions, permCloser := st.db().GetRawCollection(permissionsC)
	defer permCloser()

	findExpr := fmt.Sprintf("^cloud#.*#%s$", coreuser.UserGlobalKey(user.Id()))
	query := permissions.Find(
		bson.D{{"_id", bson.D{{"$regex", findExpr}}}},
	).Batch(100)

	var doc permissionDoc
	iter := query.Iter()
	var cloudNames []string
	for iter.Next(&doc) {
		cloudName := strings.TrimPrefix(doc.ObjectGlobalKey, "cloud#")
		cloudNames = append(cloudNames, cloudName)
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return cloudNames, nil
}

// fillInCloudUserAccess fills in the Access rights for this user on the clouds (but not other users).
func (st *State) fillInCloudUserAccess(user names.UserTag, cloudInfo []cloud.CloudAccess) error {
	// Note: Even for Superuser we track the individual Access for each model.
	username := strings.ToLower(user.Name())
	var permissionIds []string
	for _, info := range cloudInfo {
		permId := permissionID(cloudGlobalKey(info.Name), coreuser.UserGlobalKey(username))
		permissionIds = append(permissionIds, permId)
	}

	// Record index by name so we can fill access details below.
	indexByName := make(map[string]int, len(cloudInfo))
	for i, info := range cloudInfo {
		indexByName[info.Name] = i
	}

	perms, closer := st.db().GetCollection(permissionsC)
	defer closer()
	query := perms.Find(bson.M{"_id": bson.M{"$in": permissionIds}}).Batch(100)
	iter := query.Iter()

	var doc permissionDoc
	for iter.Next(&doc) {
		cloudName := strings.TrimPrefix(doc.ObjectGlobalKey, "cloud#")
		cloudIdx := indexByName[cloudName]

		details := &cloudInfo[cloudIdx]
		access := permission.Access(doc.Access)
		if err := access.Validate(); err == nil {
			details.Access = access
		}
	}
	return iter.Close()
}
