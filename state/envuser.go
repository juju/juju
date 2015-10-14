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
}

// envUserLastConnectionDoc is updated by the apiserver whenever the user
// connects over the API. This update is not done using mgo.txn so the values
// could well change underneath a normal transaction and as such, it should
// NEVER appear in any transaction asserts. It is really informational only as
// far as everyone except the api server is concerned.
type envUserLastConnectionDoc struct {
	ID             string    `bson:"_id"`
	EnvUUID        string    `bson:"env-uuid"`
	UserName       string    `bson:"user"`
	LastConnection time.Time `bson:"last-connection"`
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

// LastConnection returns when this EnvironmentUser last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (e *EnvironmentUser) LastConnection() (time.Time, error) {
	lastConnections, closer := e.st.getRawCollection(envUserLastConnectionC)
	defer closer()

	username := strings.ToLower(e.UserName())
	var lastConn envUserLastConnectionDoc
	err := lastConnections.FindId(e.st.docID(username)).Select(bson.D{{"last-connection", 1}}).One(&lastConn)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = errors.Wrap(err, NeverConnectedError(e.UserName()))
		}
		return time.Time{}, errors.Trace(err)
	}

	return lastConn.LastConnection.UTC(), nil
}

// NeverConnectedError is used to indicate that a user has never connected to
// an environment.
type NeverConnectedError string

// Error returns the error string for a user who has never connected to an
// environment.
func (e NeverConnectedError) Error() string {
	return `never connected: "` + string(e) + `"`
}

// IsNeverConnectedError returns true if err is of type NeverConnectedError.
func IsNeverConnectedError(err error) bool {
	_, ok := errors.Cause(err).(NeverConnectedError)
	return ok
}

// UpdateLastConnection updates the last connection time of the environment user.
func (e *EnvironmentUser) UpdateLastConnection() error {
	lastConnections, closer := e.st.getCollection(envUserLastConnectionC)
	defer closer()

	lastConnectionsW := lastConnections.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastConnectionsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastConn := envUserLastConnectionDoc{
		ID:             e.st.docID(strings.ToLower(e.UserName())),
		EnvUUID:        e.EnvironmentTag().Id(),
		UserName:       e.UserName(),
		LastConnection: nowToTheSecond(),
	}
	_, err := lastConnectionsW.UpsertId(lastConn.ID, lastConn)
	return errors.Trace(err)
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
	op := createEnvUserOp(envuuid, user, createdBy, displayName)
	err := st.runTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("environment user %q", user.Username())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Re-read from DB to get the multi-env updated values.
	return st.EnvironmentUser(user)
}

// envUserID returns the document id of the environment user
func envUserID(user names.UserTag) string {
	username := user.Username()
	return strings.ToLower(username)
}

func createEnvUserOp(envuuid string, user, createdBy names.UserTag, displayName string) txn.Op {
	creatorname := createdBy.Username()
	doc := &envUserDoc{
		ID:          envUserID(user),
		EnvUUID:     envuuid,
		UserName:    user.Username(),
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: nowToTheSecond(),
	}
	return txn.Op{
		C:      envUsersC,
		Id:     envUserID(user),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// RemoveEnvironmentUser removes a user from the database.
func (st *State) RemoveEnvironmentUser(user names.UserTag) error {
	ops := []txn.Op{{
		C:      envUsersC,
		Id:     envUserID(user),
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

// UserEnvironment contains information about an environment that a
// user has access to.
type UserEnvironment struct {
	*Environment
	User names.UserTag
}

// LastConnection returns the last time the user has connected to the
// environment.
func (e *UserEnvironment) LastConnection() (time.Time, error) {
	lastConnections, lastConnCloser := e.st.getRawCollection(envUserLastConnectionC)
	defer lastConnCloser()

	lastConnDoc := envUserLastConnectionDoc{}
	id := ensureEnvUUID(e.EnvironTag().Id(), strings.ToLower(e.User.Username()))
	err := lastConnections.FindId(id).Select(bson.D{{"last-connection", 1}}).One(&lastConnDoc)
	if (err != nil && err != mgo.ErrNotFound) || lastConnDoc.LastConnection.IsZero() {
		return time.Time{}, errors.Trace(NeverConnectedError(e.User.Username()))
	}

	return lastConnDoc.LastConnection, nil
}

// EnvironmentsForUser returns a list of enviroments that the user
// is able to access.
func (st *State) EnvironmentsForUser(user names.UserTag) ([]*UserEnvironment, error) {
	// Since there are no groups at this stage, the simplest way to get all
	// the environments that a particular user can see is to look through the
	// environment user collection. A raw collection is required to support
	// queries across multiple environments.
	envUsers, userCloser := st.getRawCollection(envUsersC)
	defer userCloser()

	// TODO: consider adding an index to the envUsers collection on the username.
	var userSlice []envUserDoc
	err := envUsers.Find(bson.D{{"user", user.Username()}}).Select(bson.D{{"env-uuid", 1}, {"_id", 1}}).All(&userSlice)
	if err != nil {
		return nil, err
	}

	var result []*UserEnvironment
	for _, doc := range userSlice {
		envTag := names.NewEnvironTag(doc.EnvUUID)
		env, err := st.GetEnvironment(envTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		result = append(result, &UserEnvironment{Environment: env, User: user})
	}

	return result, nil
}

// IsSystemAdministrator returns true if the user specified has access to the
// state server environment (the system environment).
func (st *State) IsSystemAdministrator(user names.UserTag) (bool, error) {
	ssinfo, err := st.StateServerInfo()
	if err != nil {
		return false, errors.Annotate(err, "could not get state server info")
	}

	serverUUID := ssinfo.EnvironmentTag.Id()

	envUsers, userCloser := st.getRawCollection(envUsersC)
	defer userCloser()

	count, err := envUsers.Find(bson.D{
		{"env-uuid", serverUUID},
		{"user", user.Username()},
	}).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return count == 1, nil
}
