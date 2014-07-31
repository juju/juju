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

// EnvironmentUser represents a user within an environment
// Whereas the user could represent a remote user or a user
// across multiple environments the environment user always represents
// a single user for a single environment.
type EnvironmentUser struct {
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

// ID returns the ID of the environment user.
func (e *EnvironmentUser) ID() string {
	return e.doc.ID
}

// EnvUUID Returns the environment UUIID of the environment user.
func (e *EnvironmentUser) EnvUUID() string {
	return e.doc.EnvUUID
}

// UserName returns the user name of the environment user.
func (e *EnvironmentUser) UserName() string {
	return e.doc.User
}

// Alias returns the alias of the environment user.
func (e *EnvironmentUser) Alias() string {
	return e.doc.Alias
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

func envUserID(envuuid, user string) string {
	return fmt.Sprintf("%s:%s", envuuid, user)
}

func (st *State) getEnvironmentUser(envuuid, user string, doc *envUserDoc) error {
	envUsers, closer := st.getCollection(envUsersC)
	defer closer()
	id := envUserID(envuuid, user)
	err := envUsers.Find(bson.D{{"_id", id}}).One(doc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("envUser %q", user)
	}
	return err
}

// EnvironmentUser returns the environment user for the given envuuid and user.
func (st *State) EnvironmentUser(envuuid, user string) (*EnvironmentUser, error) {
	envUser := &EnvironmentUser{st: st}
	if err := st.getEnvironmentUser(envuuid, user, &envUser.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return envUser, nil
}

// Adds a new EnvironmentUser to the database
func (st *State) AddEnvironmentUser(envuuid, user, displayName, alias, createdBy string) (*EnvironmentUser, error) {
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
	if !names.IsValidEnvironment(envuuid) {
		return nil, errors.Errorf("invalid environment %q", envuuid)
	}

	id := envUserID(envuuid, user)
	envUser := &EnvironmentUser{
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
