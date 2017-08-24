// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/permission"
)

// modelUserLastConnectionDoc is updated by the apiserver whenever the user
// connects over the API. This update is not done using mgo.txn so the values
// could well change underneath a normal transaction and as such, it should
// NEVER appear in any transaction asserts. It is really informational only as
// far as everyone except the api server is concerned.
type modelUserLastConnectionDoc struct {
	ID             string    `bson:"_id"`
	ModelUUID      string    `bson:"model-uuid"`
	UserName       string    `bson:"user"`
	LastConnection time.Time `bson:"last-connection"`
}

// setModelAccess changes the user's access permissions on the model.
func (m *Model) setModelAccess(access permission.Access, userGlobalKey, modelUUID string) error {
	if err := permission.ValidateModelAccess(access); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(modelKey(modelUUID), userGlobalKey, access)
	err := m.st.db().RunTransactionFor(modelUUID, []txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// LastModelConnection returns when this User last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (m *Model) LastModelConnection(user names.UserTag) (time.Time, error) {
	lastConnections, closer := m.st.db().GetRawCollection(modelUserLastConnectionC)
	defer closer()

	username := user.Id()
	var lastConn modelUserLastConnectionDoc
	err := lastConnections.FindId(m.st.docID(username)).Select(bson.D{{"last-connection", 1}}).One(&lastConn)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = errors.Wrap(err, NeverConnectedError(username))
		}
		return time.Time{}, errors.Trace(err)
	}

	return lastConn.LastConnection.UTC(), nil
}

// NeverConnectedError is used to indicate that a user has never connected to
// an model.
type NeverConnectedError string

// Error returns the error string for a user who has never connected to an
// model.
func (e NeverConnectedError) Error() string {
	return `never connected: "` + string(e) + `"`
}

// IsNeverConnectedError returns true if err is of type NeverConnectedError.
func IsNeverConnectedError(err error) bool {
	_, ok := errors.Cause(err).(NeverConnectedError)
	return ok
}

// UpdateLastModelConnection updates the last connection time of the model user.
func (m *Model) UpdateLastModelConnection(user names.UserTag) error {
	return m.updateLastModelConnection(user, m.st.nowToTheSecond())
}

func (m *Model) updateLastModelConnection(user names.UserTag, when time.Time) error {
	lastConnections, closer := m.st.db().GetCollection(modelUserLastConnectionC)
	defer closer()

	lastConnectionsW := lastConnections.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastConnectionsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastConn := modelUserLastConnectionDoc{
		ID:             m.st.docID(strings.ToLower(user.Id())),
		ModelUUID:      m.UUID(),
		UserName:       user.Id(),
		LastConnection: when,
	}
	_, err := lastConnectionsW.UpsertId(lastConn.ID, lastConn)
	return errors.Trace(err)
}

// ModelUser returns a userAccessDoc for the given model.
func (m *Model) modelUser(modelUUID string, user names.UserTag) (userAccessDoc, error) {
	modelUser := userAccessDoc{}
	modelUsers, closer := m.st.db().GetCollectionFor(modelUUID, modelUsersC)
	defer closer()

	username := strings.ToLower(user.Id())
	err := modelUsers.FindId(username).One(&modelUser)
	if err == mgo.ErrNotFound {
		return userAccessDoc{}, errors.NotFoundf("model user %q", username)
	}
	if err != nil {
		return userAccessDoc{}, errors.Trace(err)
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	modelUser.DateCreated = modelUser.DateCreated.UTC()
	return modelUser, nil
}

func createModelUserOps(modelUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access permission.Access) []txn.Op {
	creatorname := createdBy.Id()
	doc := &userAccessDoc{
		ID:          userAccessID(user),
		ObjectUUID:  modelUUID,
		UserName:    user.Id(),
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: dateCreated,
	}

	ops := []txn.Op{
		createPermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user)), access),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	return ops
}

func removeModelUserOps(modelUUID string, user names.UserTag) []txn.Op {
	return []txn.Op{
		removePermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user))),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocExists,
			Remove: true,
		}}
}

// removeModelUser removes a user from the database.
func (m *Model) removeModelUser(user names.UserTag) error {
	ops := removeModelUserOps(m.UUID(), user)
	err := m.st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("model user %q does not exist", user.Id()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ModelUUIDsForUser returns a list of UUIDs of models that the user
// is able to access.
// Results are sorted by (name, owner).
func (m *Model) ModelUUIDsForUser(user names.UserTag) ([]string, error) {
	// Consider the controller permissions overriding Model permission, for
	// this case the only relevant one is superuser.
	// The mgo query below wont work for superuser case because it needs at
	// least one model user per model.
	access, err := m.UserAccess(user, m.st.controllerTag)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	var modelUUIDs []string
	if access.Access == permission.SuperuserAccess {
		var err error
		modelUUIDs, err = m.st.AllModelUUIDs()
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		// Since there are no groups at this stage, the simplest way to get all
		// the models that a particular user can see is to look through the
		// model user collection. A raw collection is required to support
		// queries across multiple models.
		modelUsers, userCloser := m.st.db().GetRawCollection(modelUsersC)
		defer userCloser()

		var userSlice []userAccessDoc
		err := modelUsers.Find(bson.D{{"user", user.Id()}}).Select(bson.D{{"object-uuid", 1}, {"_id", 1}}).All(&userSlice)
		if err != nil {
			return nil, err
		}
		for _, doc := range userSlice {
			modelUUIDs = append(modelUUIDs, doc.ObjectUUID)
		}
	}

	modelsColl, close := m.st.db().GetCollection(modelsC)
	defer close()
	query := modelsColl.Find(bson.M{
		"_id":            bson.M{"$in": modelUUIDs},
		"migration-mode": bson.M{"$ne": MigrationModeImporting},
	}).Sort("name", "owner").Select(bson.M{"_id": 1})

	var docs []bson.M
	err = query.All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]string, len(docs))
	for i, doc := range docs {
		out[i] = doc["_id"].(string)
	}
	return out, nil
}

// IsControllerAdmin returns true if the user specified has Super User Access.
func (m *Model) IsControllerAdmin(user names.UserTag) (bool, error) {
	ua, err := m.UserAccess(user, m.st.ControllerTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return ua.Access == permission.SuperuserAccess, nil
}
