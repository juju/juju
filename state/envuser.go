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

const (
	envUserCollectionName = "envusers"
)

// EnvUser represents a user within an environment
type EnvUser struct {
	st  *State
	doc envUserDoc
}

type envUserDoc struct {
	ID             string `bson:"_id"`
	EnvUUID        string
	User           string
	Alias          string
	DisplayName    string
	CreatedBy      string
	DateCreated    time.Time
	LastConnection *time.Time
}

// Returns the ID of the envUser
func (e *EnvUser) ID() string {
	return e.doc.ID
}

// Returns the EnvironmentID of the envUser
func (e *EnvUser) EnvUUID() string {
	return e.doc.EnvUUID
}

// Returns the user name of the envUser
func (e *EnvUser) UserName() string {
	return e.doc.User
}

// Returns the alias of the envUser
func (e *EnvUser) Alias() string {
	return e.doc.Alias
}

// Returns the display name of the env user
func (e *EnvUser) DisplayName() string {
	return e.doc.DisplayName
}

// Returns the user who created the envUser
func (e *EnvUser) CreatedBy() string {
	return e.doc.CreatedBy
}

// Returns the date the envUser was created
func (e *EnvUser) DateCreated() time.Time {
	return e.doc.DateCreated
}

// Returns the last connection time
func (e *EnvUser) LastConnection() *time.Time {
	return e.doc.LastConnection
}

// Updates the last connection time
func (e *EnvUser) UpdateLastConnection() error {
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

func envUserID(envuuid, user string) string {
	return fmt.Sprintf("%s:%s", envuuid, user)
}

func (st *State) getEnvUser(envuuid, user string, doc *envUserDoc) error {
	envUsers, closer := st.getCollection(envUsersC)
	defer closer()
	id := envUserID(envuuid, user)
	err := envUsers.Find(bson.D{{"_id", id}}).One(doc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("envUser %q", user)
	}
	return err
}

// Returns the EnvUser for the given envuuid and user
func (st *State) EnvUser(envuuid, user string) (*EnvUser, error) {
	envUser := &EnvUser{st: st}
	if err := st.getEnvUser(envuuid, user, &envUser.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return envUser, nil
}

// Adds a new EnvUser to the database
func (st *State) AddEnvUser(envuuid, user, displayName, alias, createdBy string) (*EnvUser, error) {
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
	if !names.IsValidEnvironment(envuuid) {
		return nil, errors.Errorf("invalid environment %q", envuuid)
	}

	id := envUserID(envuuid, user)
	envUser := &EnvUser{
		st: st,
		doc: envUserDoc{
			ID:          id,
			EnvUUID:     envuuid,
			User:        user,
			Alias:       alias,
			DisplayName: displayName,
			CreatedBy:   createdBy,
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
