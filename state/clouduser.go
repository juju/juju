// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/mongo"
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

// CloudInfo describes interesting information for a given cloud.
type CloudInfo struct {
	cloud.Cloud

	// Access is the access level the supplied user has on this cloud.
	Access permission.Access
}

// CloudsForUser returns details including access level of clouds which can
// be seen by the specified user, or all users if the caller is a superuser.
func (st *State) CloudsForUser(user names.UserTag, all bool) ([]CloudInfo, error) {
	// We only treat the user as a superuser if they pass --all
	isControllerSuperuser := false
	if all {
		var err error
		isControllerSuperuser, err = st.isUserSuperuser(user)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	clouds, closer := st.db().GetCollection(cloudsC)
	defer closer()

	var cloudQuery mongo.Query
	if isControllerSuperuser {
		// Fast path, we just get all the clouds.
		cloudQuery = clouds.Find(nil)
	} else {
		cloudNames, err := st.cloudNamesForUser(user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudQuery = clouds.Find(bson.M{
			"_id": bson.M{"$in": cloudNames},
		})
	}
	cloudQuery = cloudQuery.Sort("name")

	var cloudDocs []cloudDoc
	if err := cloudQuery.All(&cloudDocs); err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]CloudInfo, len(cloudDocs))
	for i, c := range cloudDocs {
		result[i] = CloudInfo{
			Cloud: c.toCloud(),
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

	findExpr := fmt.Sprintf("^.*#%s$", userGlobalKey(user.Id()))
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
func (st *State) fillInCloudUserAccess(user names.UserTag, cloudInfo []CloudInfo) error {
	// Note: Even for Superuser we track the individual Access for each model.
	username := strings.ToLower(user.Name())
	var permissionIds []string
	for _, info := range cloudInfo {
		permId := permissionID(cloudGlobalKey(info.Name), userGlobalKey(username))
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
