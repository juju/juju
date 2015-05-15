// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// EnvironmentUser represents a user access to an environment whereas the user
// could represent a remote user or a user across multiple environments the
// environment user always represents a single user for a single environment.
// There should be no more than one EnvironmentUser per environment.
type EnvironmentUser struct {
	st  *State
	doc envUserDoc
}

type envUserDoc struct {
	ID          string    `bson:"_id"`
	EnvUUID     string    `bson:"env-uuid"`
	UserName    string    `bson:"user"`
	DisplayName string    `bson:"displayname"`
	CreatedBy   string    `bson:"createdby"`
	DateCreated time.Time `bson:"datecreated"`
	// LastConnection is updated by the apiserver whenever the user
	// connects over the API. This update is not done using mgo.txn
	// so this value could well change underneath a normal transaction
	// and as such, it should NEVER appear in any transaction asserts.
	// It is really informational only as far as everyone except the
	// api server is concerned.
	LastConnection *time.Time `bson:"lastconnection"`
}

// ID returns the ID of the environment user.
func (e *EnvironmentUser) ID() string {
	return e.doc.ID
}

// EnvironmentTag returns the environment tag of the environment user.
func (e *EnvironmentUser) EnvironmentTag() names.EnvironTag {
	return names.NewEnvironTag(e.doc.EnvUUID)
}

// UserTag returns the tag for the environment user.
func (e *EnvironmentUser) UserTag() names.UserTag {
	return names.NewUserTag(e.doc.UserName)
}

// UserName returns the user name of the environment user.
func (e *EnvironmentUser) UserName() string {
	return e.doc.UserName
}

// DisplayName returns the display name of the environment user.
func (e *EnvironmentUser) DisplayName() string {
	return e.doc.DisplayName
}

// CreatedBy returns the user who created the environment user.
func (e *EnvironmentUser) CreatedBy() string {
	return e.doc.CreatedBy
}

// DateCreated returns the date the environment user was created in UTC.
func (e *EnvironmentUser) DateCreated() time.Time {
	return e.doc.DateCreated.UTC()
}

// LastLogin returns when this EnvironmentUser last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (e *EnvironmentUser) LastConnection() *time.Time {
	when := e.doc.LastConnection
	if when == nil {
		return nil
	}
	result := when.UTC()
	return &result
}

// UpdateLastConnection updates the last connection time of the environment user.
func (e *EnvironmentUser) UpdateLastConnection() error {
	envUsers, closer := e.st.getCollection(envUsersC)
	defer closer()
	// Update the safe mode of the underlying session to be not require
	// write majority, nor sync to disk.
	session := envUsers.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	timestamp := nowToTheSecond()
	update := bson.D{{"$set", bson.D{{"lastconnection", timestamp}}}}

	id := strings.ToLower(e.UserName())
	if err := envUsers.UpdateId(id, update); err != nil {
		return errors.Annotatef(err, "cannot update last connection timestamp for envuser %q", e.ID())
	}

	e.doc.LastConnection = &timestamp
	return nil
}

// EnvironmentUser returns the environment user.
func (st *State) EnvironmentUser(user names.UserTag) (*EnvironmentUser, error) {
	envUser := &EnvironmentUser{st: st}
	envUsers, closer := st.getCollection(envUsersC)
	defer closer()

	username := strings.ToLower(user.Username())
	err := envUsers.FindId(username).One(&envUser.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("environment user %q", user.Username())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	envUser.doc.DateCreated = envUser.doc.DateCreated.UTC()
	return envUser, nil
}

// AddEnvironmentUser adds a new user to the database.
func (st *State) AddEnvironmentUser(user, createdBy names.UserTag, displayName string) (*EnvironmentUser, error) {
	// Ensure local user exists in state before adding them as an environment user.
	if user.IsLocal() {
		localUser, err := st.User(user)
		if err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", user.Name()))
		}
		if displayName == "" {
			displayName = localUser.DisplayName()
		}
	}

	// Ensure local createdBy user exists.
	if createdBy.IsLocal() {
		if _, err := st.User(createdBy); err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("createdBy user %q does not exist locally", createdBy.Name()))
		}
	}

	envuuid := st.EnvironUUID()
	op, doc := createEnvUserOpAndDoc(envuuid, user, createdBy, displayName)
	err := st.runTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("environment user %q", user.Username())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &EnvironmentUser{st: st, doc: *doc}, nil
}

func createEnvUserOpAndDoc(envuuid string, user, createdBy names.UserTag, displayName string) (txn.Op, *envUserDoc) {
	username := user.Username()
	usernameLowerCase := strings.ToLower(username)
	creatorname := createdBy.Username()
	doc := &envUserDoc{
		ID:          usernameLowerCase,
		EnvUUID:     envuuid,
		UserName:    username,
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: nowToTheSecond(),
	}
	op := txn.Op{
		C:      envUsersC,
		Id:     usernameLowerCase,
		Assert: txn.DocMissing,
		Insert: doc,
	}
	return op, doc
}

// RemoveEnvironmentUser adds a new user to the database.
func (st *State) RemoveEnvironmentUser(user names.UserTag) error {
	ops := []txn.Op{{
		C:      envUsersC,
		Id:     user.Username(),
		Assert: txn.DocExists,
		Remove: true,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(err, fmt.Sprintf("env user %q does not exist", user.Username()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// EnvironmentsForUser returns a list of enviroments that the user
// is able to access.
func (st *State) EnvironmentsForUser(user names.UserTag) ([]*Environment, error) {

	// Since there are no groups at this stage, the simplest way to get all
	// the environments that a particular user can see is to look through the
	// environment user collection. A raw collection is required to support
	// queries across multiple environments.
	envUsers, userCloser := st.getRawCollection(envUsersC)
	defer userCloser()

	// TODO: consider adding an index to the envUsers collection on the username.
	var userSlice []envUserDoc
	err := envUsers.Find(bson.D{{"user", user.Username()}}).All(&userSlice)
	if err != nil {
		return nil, err
	}

	var result []*Environment
	for _, doc := range userSlice {
		envTag := names.NewEnvironTag(doc.EnvUUID)
		env, err := st.GetEnvironment(envTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, env)
	}

	return result, nil
}
