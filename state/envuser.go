// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// EnvironmentUser represents a user access to an environment
// whereas the user could represent a remote user or a user
// across multiple environments the environment user always represents
// a single user for a single environment.
// There should be no more than one EnvironmentUser per user
type EnvironmentUser struct {
	st  *State
	doc envUserDoc
}

type envUserDoc struct {
	ID             string     `bson:"_id"`
	EnvUUID        string     `bson:"envuuid"`
	UserName       string     `bson:"user"`
	DisplayName    string     `bson:"displayname"`
	CreatedBy      string     `bson:"createdby"`
	DateCreated    time.Time  `bson:"datecreated"`
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

// DateCreated returns the date the environment user.
func (e *EnvironmentUser) DateCreated() time.Time {
	return e.doc.DateCreated
}

// LastConnection returns the last connection time of the environment user.
func (e *EnvironmentUser) LastConnection() *time.Time {
	return e.doc.LastConnection
}

// UpdateLastConnection updates the last connection time of the environment user.
func (e *EnvironmentUser) UpdateLastConnection() error {
	timestamp := nowToTheSecond()
	ops := []txn.Op{{
		C:      envUsersC,
		Id:     e.ID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"lastconnection", timestamp}}}},
	}}
	if err := e.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot update last connection timestamp for envuser %q", e.ID())
	}

	e.doc.LastConnection = &timestamp
	return nil
}

// envUserID returns the document id of the environment user with the
// following format uuid:username@provider.
func envUserID(envuuid, user string) string {
	return fmt.Sprintf("%s:%s", envuuid, user)
}

// EnvironmentUser returns the environment user.
func (st *State) EnvironmentUser(user names.UserTag) (*EnvironmentUser, error) {
	envUser := &EnvironmentUser{st: st}
	envUsers, closer := st.getCollection(envUsersC)
	defer closer()

	id := envUserID(st.EnvironTag().Id(), user.Username())
	err := envUsers.FindId(id).One(&envUser.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("envUser %q", user.Username())
	}
	return envUser, nil
}

// AddEnvironmentUser adds a new user to the database.
func (st *State) AddEnvironmentUser(user, createdBy names.UserTag, displayName string) (*EnvironmentUser, error) {

	// Ensure local user exists in state before adding them as an environment user.
	if user.Provider() == names.LocalProvider {
		if _, err := st.User(user.Name()); err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", user.Name()))
		}
	}

	// Ensure local createdBy user exists.
	if createdBy.Provider() == names.LocalProvider {
		if _, err := st.User(createdBy.Name()); err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("createdBy user %q does not exist locally", createdBy.Name()))
		}
	}

	username := user.Username()
	envuuid := st.EnvironTag().Id()
	id := envUserID(envuuid, username)
	envUser := &EnvironmentUser{
		st: st,
		doc: envUserDoc{
			ID:          id,
			EnvUUID:     envuuid,
			UserName:    username,
			DisplayName: displayName,
			CreatedBy:   createdBy.Username(),
			DateCreated: nowToTheSecond(),
		}}

	ops := []txn.Op{{
		C:      envUsersC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &envUser.doc,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.New("env user already exists")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return envUser, nil
}
