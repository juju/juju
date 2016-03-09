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

// ID returns the ID of the model user.
func (e *ModelUser) ID() string {
	return e.doc.ID
}

// ModelTag returns the model tag of the model user.
func (e *ModelUser) ModelTag() names.ModelTag {
	return names.NewModelTag(e.doc.ModelUUID)
}

// UserTag returns the tag for the model user.
func (e *ModelUser) UserTag() names.UserTag {
	return names.NewUserTag(e.doc.UserName)
}

// UserName returns the user name of the model user.
func (e *ModelUser) UserName() string {
	return e.doc.UserName
}

// DisplayName returns the display name of the model user.
func (e *ModelUser) DisplayName() string {
	return e.doc.DisplayName
}

// CreatedBy returns the user who created the model user.
func (e *ModelUser) CreatedBy() string {
	return e.doc.CreatedBy
}

// DateCreated returns the date the model user was created in UTC.
func (e *ModelUser) DateCreated() time.Time {
	return e.doc.DateCreated.UTC()
}

// ReadOnly returns whether or not the user has write access or only
// read access to the model.
func (e *ModelUser) ReadOnly() bool {
	return e.doc.ReadOnly
}

// LastConnection returns when this ModelUser last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (e *ModelUser) LastConnection() (time.Time, error) {
	lastConnections, closer := e.st.getRawCollection(modelUserLastConnectionC)
	defer closer()

	username := strings.ToLower(e.UserName())
	var lastConn modelUserLastConnectionDoc
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

// UpdateLastConnection updates the last connection time of the model user.
func (e *ModelUser) UpdateLastConnection() error {
	return e.updateLastConnection(nowToTheSecond())
}

func (e *ModelUser) updateLastConnection(when time.Time) error {
	lastConnections, closer := e.st.getCollection(modelUserLastConnectionC)
	defer closer()

	lastConnectionsW := lastConnections.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastConnectionsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastConn := modelUserLastConnectionDoc{
		ID:             e.st.docID(strings.ToLower(e.UserName())),
		ModelUUID:      e.ModelTag().Id(),
		UserName:       e.UserName(),
		LastConnection: when,
	}
	_, err := lastConnectionsW.UpsertId(lastConn.ID, lastConn)
	return errors.Trace(err)
}

// ModelUser returns the model user.
func (st *State) ModelUser(user names.UserTag) (*ModelUser, error) {
	modelUser := &ModelUser{st: st}
	modelUsers, closer := st.getCollection(modelUsersC)
	defer closer()

	username := strings.ToLower(user.Canonical())
	err := modelUsers.FindId(username).One(&modelUser.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("model user %q", user.Canonical())
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	modelUser.doc.DateCreated = modelUser.doc.DateCreated.UTC()
	return modelUser, nil
}

// ModelUserSpec defines the attributes that can be set when adding a new
// model user.
type ModelUserSpec struct {
	User        names.UserTag
	CreatedBy   names.UserTag
	DisplayName string
	ReadOnly    bool
}

// AddModelUser adds a new user to the database.
func (st *State) AddModelUser(spec ModelUserSpec) (*ModelUser, error) {
	// Ensure local user exists in state before adding them as an model user.
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

	modelUUID := st.ModelUUID()
	op := createModelUserOp(modelUUID, spec.User, spec.CreatedBy, spec.DisplayName, nowToTheSecond(), spec.ReadOnly)
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

// modelUserID returns the document id of the model user
func modelUserID(user names.UserTag) string {
	username := user.Canonical()
	return strings.ToLower(username)
}

func createModelUserOp(modelUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, readOnly bool) txn.Op {
	creatorname := createdBy.Canonical()
	doc := &modelUserDoc{
		ID:          modelUserID(user),
		ModelUUID:   modelUUID,
		UserName:    user.Canonical(),
		DisplayName: displayName,
		ReadOnly:    readOnly,
		CreatedBy:   creatorname,
		DateCreated: dateCreated,
	}
	return txn.Op{
		C:      modelUsersC,
		Id:     modelUserID(user),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// RemoveModelUser removes a user from the database.
func (st *State) RemoveModelUser(user names.UserTag) error {
	ops := []txn.Op{{
		C:      modelUsersC,
		Id:     modelUserID(user),
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

// UserModel contains information about an model that a
// user has access to.
type UserModel struct {
	*Model
	User names.UserTag
}

// LastConnection returns the last time the user has connected to the
// model.
func (e *UserModel) LastConnection() (time.Time, error) {
	lastConnections, lastConnCloser := e.st.getRawCollection(modelUserLastConnectionC)
	defer lastConnCloser()

	lastConnDoc := modelUserLastConnectionDoc{}
	id := ensureModelUUID(e.ModelTag().Id(), strings.ToLower(e.User.Canonical()))
	err := lastConnections.FindId(id).Select(bson.D{{"last-connection", 1}}).One(&lastConnDoc)
	if (err != nil && err != mgo.ErrNotFound) || lastConnDoc.LastConnection.IsZero() {
		return time.Time{}, errors.Trace(NeverConnectedError(e.User.Canonical()))
	}

	return lastConnDoc.LastConnection, nil
}

// ModelsForUser returns a list of models that the user
// is able to access.
func (st *State) ModelsForUser(user names.UserTag) ([]*UserModel, error) {
	// Since there are no groups at this stage, the simplest way to get all
	// the models that a particular user can see is to look through the
	// model user collection. A raw collection is required to support
	// queries across multiple models.
	modelUsers, userCloser := st.getRawCollection(modelUsersC)
	defer userCloser()

	// TODO: consider adding an index to the modelUsers collection on the username.
	var userSlice []modelUserDoc
	err := modelUsers.Find(bson.D{{"user", user.Canonical()}}).Select(bson.D{{"model-uuid", 1}, {"_id", 1}}).All(&userSlice)
	if err != nil {
		return nil, err
	}

	var result []*UserModel
	for _, doc := range userSlice {
		modelTag := names.NewModelTag(doc.ModelUUID)
		env, err := st.GetModel(modelTag)
		if err != nil {
			return nil, errors.Trace(err)
		}

		result = append(result, &UserModel{Model: env, User: user})
	}

	return result, nil
}

// IsControllerAdministrator returns true if the user specified has access to the
// controller model (the system model).
func (st *State) IsControllerAdministrator(user names.UserTag) (bool, error) {
	ssinfo, err := st.ControllerInfo()
	if err != nil {
		return false, errors.Annotate(err, "could not get controller info")
	}

	serverUUID := ssinfo.ModelTag.Id()

	modelUsers, userCloser := st.getRawCollection(modelUsersC)
	defer userCloser()

	count, err := modelUsers.Find(bson.D{
		{"model-uuid", serverUUID},
		{"user", user.Canonical()},
	}).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return count == 1, nil
}
