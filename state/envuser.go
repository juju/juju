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

// ModelUser represents a user access to an model whereas the user
// could represent a remote user or a user across multiple models the
// model user always represents a single user for a single model.
// There should be no more than one ModelUser per model.
type ModelUser struct {
	st  *State
	doc modelUserDoc
}

type modelUserDoc struct {
	ID          string    `bson:"_id"`
	ModelUUID   string    `bson:"model-uuid"`
	UserName    string    `bson:"user"`
	DisplayName string    `bson:"displayname"`
	CreatedBy   string    `bson:"createdby"`
	DateCreated time.Time `bson:"datecreated"`
	ReadOnly    bool      `bson:"readonly"`
}

// envUserLastConnectionDoc is updated by the apiserver whenever the user
// connects over the API. This update is not done using mgo.txn so the values
// could well change underneath a normal transaction and as such, it should
// NEVER appear in any transaction asserts. It is really informational only as
// far as everyone except the api server is concerned.
type envUserLastConnectionDoc struct {
	ID             string    `bson:"_id"`
	ModelUUID      string    `bson:"model-uuid"`
	UserName       string    `bson:"user"`
	LastConnection time.Time `bson:"last-connection"`
}

// ID returns the ID of the environment user.
func (e *ModelUser) ID() string {
	return e.doc.ID
}

// ModelTag returns the environment tag of the environment user.
func (e *ModelUser) ModelTag() names.ModelTag {
	return names.NewModelTag(e.doc.ModelUUID)
}

// UserTag returns the tag for the environment user.
func (e *ModelUser) UserTag() names.UserTag {
	return names.NewUserTag(e.doc.UserName)
}

// UserName returns the user name of the environment user.
func (e *ModelUser) UserName() string {
	return e.doc.UserName
}

// DisplayName returns the display name of the environment user.
func (e *ModelUser) DisplayName() string {
	return e.doc.DisplayName
}

// CreatedBy returns the user who created the environment user.
func (e *ModelUser) CreatedBy() string {
	return e.doc.CreatedBy
}

// DateCreated returns the date the environment user was created in UTC.
func (e *ModelUser) DateCreated() time.Time {
	return e.doc.DateCreated.UTC()
}

// ReadOnly returns whether or not the user has write access or only
// read access to the environment.
func (e *ModelUser) ReadOnly() bool {
	return e.doc.ReadOnly
}

// LastConnection returns when this ModelUser last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (e *ModelUser) LastConnection() (time.Time, error) {
	lastConnections, closer := e.st.getRawCollection(modelUserLastConnectionC)
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
func (e *ModelUser) UpdateLastConnection() error {
	lastConnections, closer := e.st.getCollection(modelUserLastConnectionC)
	defer closer()

	lastConnectionsW := lastConnections.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastConnectionsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastConn := envUserLastConnectionDoc{
		ID:             e.st.docID(strings.ToLower(e.UserName())),
		ModelUUID:      e.ModelTag().Id(),
		UserName:       e.UserName(),
		LastConnection: nowToTheSecond(),
	}
	_, err := lastConnectionsW.UpsertId(lastConn.ID, lastConn)
	return errors.Trace(err)
}

// ModelUser returns the environment user.
func (st *State) ModelUser(user names.UserTag) (*ModelUser, error) {
	envUser := &ModelUser{st: st}
	envUsers, closer := st.getCollection(modelUsersC)
	defer closer()

	username := strings.ToLower(user.Canonical())
	err := envUsers.FindId(username).One(&envUser.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("model user %q", user.Canonical())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	envUser.doc.DateCreated = envUser.doc.DateCreated.UTC()
	return envUser, nil
}

// EnvModelSpec defines the attributes that can be set when adding a new
// model user.
type EnvModelSpec struct {
	User        names.UserTag
	CreatedBy   names.UserTag
	DisplayName string
	ReadOnly    bool
}

// AddModelUser adds a new user to the database.
func (st *State) AddModelUser(spec EnvModelSpec) (*ModelUser, error) {
	// Ensure local user exists in state before adding them as an environment user.
	if spec.User.IsLocal() {
		localUser, err := st.User(spec.User)
		if err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", spec.User.Name()))
		}
		if spec.DisplayName == "" {
			spec.DisplayName = localUser.DisplayName()
		}
	}

	// Ensure local createdBy user exists.
	if spec.CreatedBy.IsLocal() {
		if _, err := st.User(spec.CreatedBy); err != nil {
			return nil, errors.Annotatef(err, "createdBy user %q does not exist locally", spec.CreatedBy.Name())
		}
	}

	envuuid := st.ModelUUID()
	op := createEnvUserOp(envuuid, spec.User, spec.CreatedBy, spec.DisplayName, spec.ReadOnly)
	err := st.runTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("model user %q", spec.User.Canonical())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Re-read from DB to get the multi-env updated values.
	return st.ModelUser(spec.User)
}

// envUserID returns the document id of the environment user
func envUserID(user names.UserTag) string {
	username := user.Canonical()
	return strings.ToLower(username)
}

func createEnvUserOp(envuuid string, user, createdBy names.UserTag, displayName string, readOnly bool) txn.Op {
	creatorname := createdBy.Canonical()
	doc := &modelUserDoc{
		ID:          envUserID(user),
		ModelUUID:   envuuid,
		UserName:    user.Canonical(),
		DisplayName: displayName,
		ReadOnly:    readOnly,
		CreatedBy:   creatorname,
		DateCreated: nowToTheSecond(),
	}
	return txn.Op{
		C:      modelUsersC,
		Id:     envUserID(user),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// RemoveModelUser removes a user from the database.
func (st *State) RemoveModelUser(user names.UserTag) error {
	ops := []txn.Op{{
		C:      modelUsersC,
		Id:     envUserID(user),
		Assert: txn.DocExists,
		Remove: true,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(err, fmt.Sprintf("env user %q does not exist", user.Canonical()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// UserModel contains information about an environment that a
// user has access to.
type UserModel struct {
	*Model
	User names.UserTag
}

// LastConnection returns the last time the user has connected to the
// environment.
func (e *UserModel) LastConnection() (time.Time, error) {
	lastConnections, lastConnCloser := e.st.getRawCollection(modelUserLastConnectionC)
	defer lastConnCloser()

	lastConnDoc := envUserLastConnectionDoc{}
	id := ensureEnvUUID(e.ModelTag().Id(), strings.ToLower(e.User.Canonical()))
	err := lastConnections.FindId(id).Select(bson.D{{"last-connection", 1}}).One(&lastConnDoc)
	if (err != nil && err != mgo.ErrNotFound) || lastConnDoc.LastConnection.IsZero() {
		return time.Time{}, errors.Trace(NeverConnectedError(e.User.Canonical()))
	}

	return lastConnDoc.LastConnection, nil
}

// EnvironmentsForUser returns a list of enviroments that the user
// is able to access.
func (st *State) EnvironmentsForUser(user names.UserTag) ([]*UserModel, error) {
	// Since there are no groups at this stage, the simplest way to get all
	// the environments that a particular user can see is to look through the
	// environment user collection. A raw collection is required to support
	// queries across multiple environments.
	envUsers, userCloser := st.getRawCollection(modelUsersC)
	defer userCloser()

	// TODO: consider adding an index to the envUsers collection on the username.
	var userSlice []modelUserDoc
	err := envUsers.Find(bson.D{{"user", user.Canonical()}}).Select(bson.D{{"model-uuid", 1}, {"_id", 1}}).All(&userSlice)
	if err != nil {
		return nil, err
	}

	var result []*UserModel
	for _, doc := range userSlice {
		modelTag := names.NewModelTag(doc.ModelUUID)
		env, err := st.GetEnvironment(modelTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		result = append(result, &UserModel{Model: env, User: user})
	}

	return result, nil
}

// IsControllerAdministrator returns true if the user specified has access to the
// state server environment (the system environment).
func (st *State) IsControllerAdministrator(user names.UserTag) (bool, error) {
	ssinfo, err := st.StateServerInfo()
	if err != nil {
		return false, errors.Annotate(err, "could not get state server info")
	}

	serverUUID := ssinfo.ModelTag.Id()

	envUsers, userCloser := st.getRawCollection(modelUsersC)
	defer userCloser()

	count, err := envUsers.Find(bson.D{
		{"model-uuid", serverUUID},
		{"user", user.Canonical()},
	}).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return count == 1, nil
}
