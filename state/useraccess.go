// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/errors"
	"github.com/juju/juju/core/description"
)

type userAccessDoc struct {
	ID          string    `bson:"_id"`
	ObjectUUID  string    `bson:"object-uuid"`
	UserName    string    `bson:"user"`
	DisplayName string    `bson:"displayname"`
	CreatedBy   string    `bson:"createdby"`
	DateCreated time.Time `bson:"datecreated"`
}

// UserAccessSpec defines the attributes that can be set when adding a new
// user access.
type UserAccessSpec struct {
	User        names.UserTag
	CreatedBy   names.UserTag
	DisplayName string
	Access      description.Access
}

// userAccessTarget defines the target of a user access granting.
type userAccessTarget struct {
	uuid      string
	globalKey string
}

// AddModelUser adds a new user for the model identified by modelUUID to the database.
func (st *State) AddModelUser(modelUUID string, spec UserAccessSpec) (description.UserAccess, error) {
	if err := description.ValidateModelAccess(spec.Access); err != nil {
		return description.UserAccess{}, errors.Annotate(err, "adding model user")
	}
	target := userAccessTarget{
		uuid:      modelUUID,
		globalKey: modelGlobalKey,
	}
	return st.addUserAccess(spec, target)
}

// AddControllerUser adds a new user for the curent controller to the database.
func (st *State) AddControllerUser(spec UserAccessSpec) (description.UserAccess, error) {
	if err := description.ValidateControllerAccess(spec.Access); err != nil {
		return description.UserAccess{}, errors.Annotate(err, "adding controller user")
	}
	return st.addUserAccess(spec, userAccessTarget{globalKey: controllerGlobalKey})
}

func (st *State) addUserAccess(spec UserAccessSpec, target userAccessTarget) (description.UserAccess, error) {
	// Ensure local user exists in state before adding them as an model user.
	if spec.User.IsLocal() {
		localUser, err := st.User(spec.User)
		if err != nil {
			return description.UserAccess{}, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", spec.User.Name()))
		}
		if spec.DisplayName == "" {
			spec.DisplayName = localUser.DisplayName()
		}
	}

	// Ensure local createdBy user exists.
	if spec.CreatedBy.IsLocal() {
		if _, err := st.User(spec.CreatedBy); err != nil {
			return description.UserAccess{}, errors.Annotatef(err, "createdBy user %q does not exist locally", spec.CreatedBy.Name())
		}
	}
	var (
		ops       []txn.Op
		err       error
		targetTag names.Tag
	)
	switch target.globalKey {
	case modelGlobalKey:
		ops = createModelUserOps(
			target.uuid,
			spec.User,
			spec.CreatedBy,
			spec.DisplayName,
			nowToTheSecond(),
			spec.Access)
		targetTag = names.NewModelTag(target.uuid)
	case controllerGlobalKey:
		ops = createControllerUserOps(
			st.ControllerUUID(),
			spec.User,
			spec.CreatedBy,
			spec.DisplayName,
			nowToTheSecond(),
			spec.Access)
		targetTag = st.controllerTag
	default:
		return description.UserAccess{}, errors.NotSupportedf("user access global key %q", target.globalKey)
	}
	err = st.runTransactionFor(target.uuid, ops)
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("user access %q", spec.User.Canonical())
	}
	if err != nil {
		return description.UserAccess{}, errors.Trace(err)
	}
	return st.UserAccess(spec.User, targetTag)
}

// userAccessID returns the document id of the user access.
func userAccessID(user names.UserTag) string {
	username := user.Canonical()
	return strings.ToLower(username)
}

// NewModelUserAccess returns a new description.UserAccess for the given userDoc and
// current Model.
func NewModelUserAccess(st *State, userDoc userAccessDoc) (description.UserAccess, error) {
	perm, err := st.userPermission(modelKey(userDoc.ObjectUUID), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return description.UserAccess{}, errors.Annotate(err, "obtaining model permission")
	}
	return newUserAccess(perm, userDoc, names.NewModelTag(userDoc.ObjectUUID)), nil
}

// NewControllerUserAccess returns a new description.UserAccess for the given userDoc and
// current Controller.
func NewControllerUserAccess(st *State, userDoc userAccessDoc) (description.UserAccess, error) {
	perm, err := st.controllerUserPermission(controllerKey(st.ControllerUUID()), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return description.UserAccess{}, errors.Annotate(err, "obtaining controller permission")
	}
	return newUserAccess(perm, userDoc, names.NewControllerTag(userDoc.ObjectUUID)), nil
}

func newUserAccess(perm *permission, userDoc userAccessDoc, object names.Tag) description.UserAccess {
	return description.UserAccess{
		UserID:      userDoc.ID,
		UserTag:     names.NewUserTag(userDoc.UserName),
		Object:      object,
		Access:      perm.access(),
		CreatedBy:   names.NewUserTag(userDoc.CreatedBy),
		DateCreated: userDoc.DateCreated.UTC(),
		DisplayName: userDoc.DisplayName,
		UserName:    userDoc.UserName,
	}
}

// UserAccess returns a new description.UserAccess for the passed subject and target.
func (st *State) UserAccess(subject names.UserTag, target names.Tag) (description.UserAccess, error) {
	if subject.IsLocal() {
		_, err := st.User(subject)
		if err != nil {
			return description.UserAccess{}, errors.Trace(err)
		}
	}

	var (
		userDoc userAccessDoc
		err     error
	)
	switch target.Kind() {
	case names.ModelTagKind:
		userDoc, err = st.modelUser(target.Id(), subject)
		if err == nil {
			return NewModelUserAccess(st, userDoc)
		}
	case names.ControllerTagKind:
		userDoc, err = st.controllerUser(subject)
		if err == nil {
			return NewControllerUserAccess(st, userDoc)
		}
	default:
		return description.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	return description.UserAccess{}, errors.Trace(err)
}

// SetUserAccess sets <access> level on <target> to <subject>.
func (st *State) SetUserAccess(subject names.UserTag, target names.Tag, access description.Access) (description.UserAccess, error) {
	err := access.Validate()
	if err != nil {
		return description.UserAccess{}, errors.Trace(err)
	}
	switch target.Kind() {
	case names.ModelTagKind:
		err = st.setModelAccess(access, userGlobalKey(userAccessID(subject)), target.Id())
	case names.ControllerTagKind:
		err = st.setControllerAccess(access, userGlobalKey(userAccessID(subject)))
	default:
		return description.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	if err != nil {
		return description.UserAccess{}, errors.Trace(err)
	}
	return st.UserAccess(subject, target)
}

// RemoveUserAccess removes access for subject to the passed tag.
func (st *State) RemoveUserAccess(subject names.UserTag, target names.Tag) error {
	switch target.Kind() {
	case names.ModelTagKind:
		return errors.Trace(st.removeModelUser(subject))
	case names.ControllerTagKind:
		return errors.Trace(st.removeControllerUser(subject))
	}
	return errors.NotValidf("%q as a target", target.Kind())
}
