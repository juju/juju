// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v3"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

type userDocBefore struct {
	Name           string    `bson:"_id"`
	LastConnection time.Time `bson:"lastconnection"`
}

func MigrateUserLastConnectionToLastLogin(st *State) error {
	var oldDocs []userDocBefore

	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	users, closer := st.getCollection(usersC)
	defer closer()
	err = users.Find(bson.D{{
		"lastconnection", bson.D{{"$exists", true}}}}).All(&oldDocs)
	if err != nil {
		return err
	}

	var zeroTime time.Time

	ops := []txn.Op{}
	for _, oldDoc := range oldDocs {
		upgradesLogger.Debugf("updating user %q", oldDoc.Name)
		var lastLogin *time.Time
		if oldDoc.LastConnection != zeroTime {
			lastLogin = &oldDoc.LastConnection
		}
		ops = append(ops,
			txn.Op{
				C:      usersC,
				Id:     oldDoc.Name,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"lastlogin", lastLogin}}},
					{"$unset", bson.D{{"lastconnection", nil}}},
					{"$unset", bson.D{{"_id_", nil}}},
				},
			})
	}

	return st.runTransaction(ops)
}

// AddStateUsersAsEnvironUsers loops through all users stored in state and
// adds them as environment users with a local provider.
func AddStateUsersAsEnvironUsers(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	var userSlice []userDoc
	users, closer := st.getCollection(usersC)
	defer closer()

	err = users.Find(nil).All(&userSlice)
	if err != nil {
		return errors.Trace(err)
	}

	for _, uDoc := range userSlice {
		user := &User{
			st:  st,
			doc: uDoc,
		}
		uTag := user.UserTag()

		_, err := st.EnvironmentUser(uTag)
		if err != nil && errors.IsNotFound(err) {
			_, err = st.AddEnvironmentUser(uTag, uTag)
			if err != nil {
				return errors.Trace(err)
			}
		} else {
			upgradesLogger.Infof("user '%s' already added to environment", uTag.Username())
		}

	}
	return nil
}

// AddEnvironmentUUIDToStateServerDoc adds environment uuid to state server doc.
func AddEnvironmentUUIDToStateServerDoc(st *State) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}
	upgradesLogger.Debugf("adding env uuid %q", env.UUID())

	ops := []txn.Op{{
		C:      stateServersC,
		Id:     environGlobalKey,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"env-uuid", env.UUID()},
		}}},
	}}

	return st.runTransaction(ops)
}

// AddCharmStoragePaths adds storagepath fields
// to the specified charms.
func AddCharmStoragePaths(st *State, storagePaths map[*charm.URL]string) error {
	var ops []txn.Op
	for curl, storagePath := range storagePaths {
		upgradesLogger.Debugf("adding storage path %q to %s", storagePath, curl)
		op := txn.Op{
			C:      charmsC,
			Id:     curl.String(),
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"storagepath", storagePath}}},
				{"$unset", bson.D{{"bundleurl", nil}}},
			},
		}
		ops = append(ops, op)
	}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("charms")
	}
	return err
}

// SetOwnerAndServerUUIDForEnvironment adds the environment uuid as the server
// uuid as well (it is the initial environment, so all good), and the owner to
// "admin@local", again all good as all existing environments have a user
// called "admin".
func SetOwnerAndServerUUIDForEnvironment(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}
	owner := names.NewLocalUserTag("admin")
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     env.UUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"server-uuid", env.UUID()},
			{"owner", owner.Username()},
		}}},
	}}
	return st.runTransaction(ops)
}

// AddEnvUUIDToServicesID prepends the environment UUID to the ID of all service docs.
func AddEnvUUIDToServicesID(st *State) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}

	var servicesDocs []serviceDoc
	services, closer := st.getCollection(servicesC)
	defer closer()

	if err = services.Find(bson.D{{"env-uuid", ""}}).All(&servicesDocs); err != nil {
		return errors.Trace(err)
	}

	upgradesLogger.Debugf("adding env uuid %q", env.UUID())

	uuid := env.UUID()
	ops := []txn.Op{}
	for _, service := range servicesDocs {
		service.EnvUUID = uuid
		service.Name = service.DocID
		ops = append(ops,
			[]txn.Op{{
				C:      servicesC,
				Id:     service.DocID,
				Assert: txn.DocExists,
				Remove: true,
			}, {
				C: servicesC,

				// In the old serialization, _id was mapped to the Name field.
				// Now _id is mapped to DocID. As such, we have to get the old
				// doc Name from the DocID field.
				Id:     st.idForEnv(service.DocID),
				Assert: txn.DocMissing,
				Insert: service,
			}}...)
	}

	return st.runTransaction(ops)
}
