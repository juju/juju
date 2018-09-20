// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/permission"
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
	Access      permission.Access
}

// userAccessTarget defines the target of a user access granting.
type userAccessTarget struct {
	uuid      string
	globalKey string
}

// AddUser adds a new user for the model to the database.
func (m *Model) AddUser(spec UserAccessSpec) (permission.UserAccess, error) {
	if err := permission.ValidateModelAccess(spec.Access); err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "adding model user")
	}
	target := userAccessTarget{
		uuid:      m.UUID(),
		globalKey: modelGlobalKey,
	}
	return m.st.addUserAccess(spec, target)
}

// AddControllerUser adds a new user for the curent controller to the database.
func (st *State) AddControllerUser(spec UserAccessSpec) (permission.UserAccess, error) {
	if err := permission.ValidateControllerAccess(spec.Access); err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "adding controller user")
	}
	return st.addUserAccess(spec, userAccessTarget{globalKey: controllerGlobalKey})
}

func (st *State) addUserAccess(spec UserAccessSpec, target userAccessTarget) (permission.UserAccess, error) {
	// Ensure local user exists in state before adding them as an model user.
	if spec.User.IsLocal() {
		localUser, err := st.User(spec.User)
		if err != nil {
			return permission.UserAccess{}, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", spec.User.Name()))
		}
		if spec.DisplayName == "" {
			spec.DisplayName = localUser.DisplayName()
		}
	}

	// Ensure local createdBy user exists.
	if spec.CreatedBy.IsLocal() {
		if _, err := st.User(spec.CreatedBy); err != nil {
			return permission.UserAccess{}, errors.Annotatef(err, "createdBy user %q does not exist locally", spec.CreatedBy.Name())
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
			st.nowToTheSecond(),
			spec.Access)
		targetTag = names.NewModelTag(target.uuid)
	case controllerGlobalKey:
		ops = createControllerUserOps(
			st.ControllerUUID(),
			spec.User,
			spec.CreatedBy,
			spec.DisplayName,
			st.nowToTheSecond(),
			spec.Access)
		targetTag = st.controllerTag
	default:
		return permission.UserAccess{}, errors.NotSupportedf("user access global key %q", target.globalKey)
	}
	err = st.db().RunTransactionFor(target.uuid, ops)
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("user access %q", spec.User.Id())
	}
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	return st.UserAccess(spec.User, targetTag)
}

// userAccessID returns the document id of the user access.
func userAccessID(user names.UserTag) string {
	username := user.Id()
	return strings.ToLower(username)
}

// NewModelUserAccess returns a new permission.UserAccess for the given userDoc and
// current Model.
func NewModelUserAccess(st *State, userDoc userAccessDoc) (permission.UserAccess, error) {
	perm, err := st.userPermission(modelKey(userDoc.ObjectUUID), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "obtaining model permission")
	}
	return newUserAccess(perm, userDoc, names.NewModelTag(userDoc.ObjectUUID)), nil
}

// NewControllerUserAccess returns a new permission.UserAccess for the given userDoc and
// current Controller.
func NewControllerUserAccess(st *State, userDoc userAccessDoc) (permission.UserAccess, error) {
	perm, err := st.userPermission(controllerKey(st.ControllerUUID()), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "obtaining controller permission")
	}
	return newUserAccess(perm, userDoc, names.NewControllerTag(userDoc.ObjectUUID)), nil
}

// UserPermission returns the access permission for the passed subject and target.
func (st *State) UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error) {
	if err := st.userMayHaveAccess(subject); err != nil {
		return "", errors.Trace(err)
	}

	switch target.Kind() {
	case names.ModelTagKind, names.ControllerTagKind:
		access, err := st.UserAccess(subject, target)
		if err != nil {
			return "", errors.Trace(err)
		}
		return access.Access, nil
	case names.ApplicationOfferTagKind:
		offerUUID, err := applicationOfferUUID(st, target.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
		return st.GetOfferAccess(offerUUID, subject)
	case names.CloudTagKind:
		return st.GetCloudAccess(target.Id(), subject)
	default:
		return "", errors.NotValidf("%q as a target", target.Kind())
	}
}

func newUserAccess(perm *userPermission, userDoc userAccessDoc, object names.Tag) permission.UserAccess {
	return permission.UserAccess{
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

func (st *State) userMayHaveAccess(tag names.UserTag) error {
	if !tag.IsLocal() {
		// external users may have access
		return nil
	}
	localUser, err := st.User(tag)
	if err != nil {
		return errors.Trace(err)
	}
	// Since deleted users will throw an error above, we need to check whether the user has been disabled here.
	if localUser.IsDisabled() {
		return errors.Errorf("user %q is disabled", tag.Id())
	}
	return nil
}

// UserAccess returns a new permission.UserAccess for the passed subject and target.
func (st *State) UserAccess(subject names.UserTag, target names.Tag) (permission.UserAccess, error) {
	if err := st.userMayHaveAccess(subject); err != nil {
		return permission.UserAccess{}, errors.Trace(err)
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
		return permission.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	return permission.UserAccess{}, errors.Trace(err)
}

// SetUserAccess sets <access> level on <target> to <subject>.
func (st *State) SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error) {
	err := access.Validate()
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	switch target.Kind() {
	case names.ModelTagKind:
		err = st.setModelAccess(access, userGlobalKey(userAccessID(subject)), target.Id())
	case names.ControllerTagKind:
		err = st.setControllerAccess(access, userGlobalKey(userAccessID(subject)))
	default:
		return permission.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
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
