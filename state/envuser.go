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
	User           string     `bson:"user"`
	Alias          string     `bson:"alias"`
	DisplayName    string     `bson:"displayname"`
	CreatedBy      string     `bson:"createdby"`
	DateCreated    time.Time  `bson:"datecreated"`
	LastConnection *time.Time `bson:"lastconnection"`
}

// ID returns the ID of the environment user.
func (e *EnvironmentUser) ID() string {
	return e.doc.ID
}

// EnvUUID returns the environment UUIID of the environment user.
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

// Returns the document id of the environment user
func envUserID(envuuid, user string) string {
	return fmt.Sprintf("%s:%s", envuuid, user)
}

func (st *State) getEnvironmentUser(user string, doc *envUserDoc) error {
	envUsers, closer := st.getCollection(envUsersC)
	defer closer()
	id := envUserID(st.EnvironTag().String(), user)
	err := envUsers.Find(bson.D{{"_id", id}}).One(doc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("envUser %q", user)
	}
	return err
}

// EnvironmentUser returns the environment user for the given envuuid and user.
func (st *State) EnvironmentUser(user string) (*EnvironmentUser, error) {
	envUser := &EnvironmentUser{st: st}
	if err := st.getEnvironmentUser(user, &envUser.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return envUser, nil
}

// Adds a new EnvironmentUser to the database
func (st *State) AddEnvironmentUser(user, displayName, alias, createdBy string) (*EnvironmentUser, error) {
	envuuid := st.EnvironTag()
	if envuuid == names.NewEnvironTag("") {
		return nil, errors.Errorf("environment not set")
	}
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}

	id := envUserID(envuuid.String(), user)
	envUser := &EnvironmentUser{
		st: st,
		doc: envUserDoc{
			ID:          id,
			EnvUUID:     envuuid.String(),
			User:        user,
			Alias:       alias,
			DisplayName: displayName,
			CreatedBy:   createdBy,
			DateCreated: nowToTheSecond(),
		}}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		ops := []txn.Op{{
			C:      envUsersC,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: &envUser.doc,
		}}
		return ops, nil
	}
	err := st.run(buildTxn)
	if err == txn.ErrAborted {
		err = errors.New("env user already exists")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return envUser, nil
}
