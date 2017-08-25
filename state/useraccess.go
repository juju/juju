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

// AddModelUser adds a new user for the model identified by modelUUID to the database.
func (m *Model) AddModelUser(modelUUID string, spec UserAccessSpec) (permission.UserAccess, error) {
	if err := permission.ValidateModelAccess(spec.Access); err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "adding model user")
	}
	target := userAccessTarget{
		uuid:      modelUUID,
		globalKey: modelGlobalKey,
	}
	return m.addUserAccess(spec, target)
}

// AddControllerUser adds a new user for the curent controller to the database.
func (m *Model) AddControllerUser(spec UserAccessSpec) (permission.UserAccess, error) {
	if err := permission.ValidateControllerAccess(spec.Access); err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "adding controller user")
	}
	return m.addUserAccess(spec, userAccessTarget{globalKey: controllerGlobalKey})
}

func (m *Model) addUserAccess(spec UserAccessSpec, target userAccessTarget) (permission.UserAccess, error) {
	// Ensure local user exists in state before adding them as an model user.
	if spec.User.IsLocal() {
		localUser, err := m.st.User(spec.User)
		if err != nil {
			return permission.UserAccess{}, errors.Annotate(err, fmt.Sprintf("user %q does not exist locally", spec.User.Name()))
		}
		if spec.DisplayName == "" {
			spec.DisplayName = localUser.DisplayName()
		}
	}

	// Ensure local createdBy user exists.
	if spec.CreatedBy.IsLocal() {
		if _, err := m.st.User(spec.CreatedBy); err != nil {
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
			m.st.nowToTheSecond(),
			spec.Access)
		targetTag = names.NewModelTag(target.uuid)
	case controllerGlobalKey:
		ops = createControllerUserOps(
			m.st.ControllerUUID(),
			spec.User,
			spec.CreatedBy,
			spec.DisplayName,
			m.st.nowToTheSecond(),
			spec.Access)
		targetTag = m.st.controllerTag
	default:
		return permission.UserAccess{}, errors.NotSupportedf("user access global key %q", target.globalKey)
	}
	err = m.st.db().RunTransactionFor(target.uuid, ops)
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("user access %q", spec.User.Id())
	}
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	return m.UserAccess(spec.User, targetTag)
}

// userAccessID returns the document id of the user access.
func userAccessID(user names.UserTag) string {
	username := user.Id()
	return strings.ToLower(username)
}

// NewModelUserAccess returns a new permission.UserAccess for the given userDoc and
// current Model.
func NewModelUserAccess(m *Model, userDoc userAccessDoc) (permission.UserAccess, error) {
	perm, err := m.userPermission(modelKey(userDoc.ObjectUUID), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "obtaining model permission")
	}
	return newUserAccess(perm, userDoc, names.NewModelTag(userDoc.ObjectUUID)), nil
}

// NewControllerUserAccess returns a new permission.UserAccess for the given userDoc and
// current Controller.
func NewControllerUserAccess(m *Model, userDoc userAccessDoc) (permission.UserAccess, error) {
	perm, err := m.controllerUserPermission(controllerKey(m.st.ControllerUUID()), userGlobalKey(strings.ToLower(userDoc.UserName)))
	if err != nil {
		return permission.UserAccess{}, errors.Annotate(err, "obtaining controller permission")
	}
	return newUserAccess(perm, userDoc, names.NewControllerTag(userDoc.ObjectUUID)), nil
}

// UserPermission returns the access permission for the passed subject and target.
func (m *Model) UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error) {
	switch target.Kind() {
	case names.ModelTagKind, names.ControllerTagKind:
		access, err := m.UserAccess(subject, target)
		if err != nil {
			return "", errors.Trace(err)
		}
		return access.Access, nil
	case names.ApplicationOfferTagKind:
		offerUUID, err := applicationOfferUUID(m.st, target.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
		return m.st.GetOfferAccess(offerUUID, subject)
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

// UserAccess returns a new permission.UserAccess for the passed subject and target.
func (m *Model) UserAccess(subject names.UserTag, target names.Tag) (permission.UserAccess, error) {
	if subject.IsLocal() {
		_, err := m.st.User(subject)
		if err != nil {
			return permission.UserAccess{}, errors.Trace(err)
		}
	}

	var (
		userDoc userAccessDoc
		err     error
	)
	switch target.Kind() {
	case names.ModelTagKind:
		userDoc, err = m.modelUser(target.Id(), subject)
		if err == nil {
			return NewModelUserAccess(m, userDoc)
		}
	case names.ControllerTagKind:
		userDoc, err = m.st.controllerUser(subject)
		if err == nil {
			return NewControllerUserAccess(m, userDoc)
		}
	default:
		return permission.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	return permission.UserAccess{}, errors.Trace(err)
}

// SetUserAccess sets <access> level on <target> to <subject>.
func (m *Model) SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error) {
	err := access.Validate()
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	switch target.Kind() {
	case names.ModelTagKind:
		err = m.setModelAccess(access, userGlobalKey(userAccessID(subject)), target.Id())
	case names.ControllerTagKind:
		err = m.st.setControllerAccess(access, userGlobalKey(userAccessID(subject)))
	default:
		return permission.UserAccess{}, errors.NotValidf("%q as a target", target.Kind())
	}
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	return m.UserAccess(subject, target)
}

// RemoveUserAccess removes access for subject to the passed tag.
func (m *Model) RemoveUserAccess(subject names.UserTag, target names.Tag) error {
	switch target.Kind() {
	case names.ModelTagKind:
		return errors.Trace(m.removeModelUser(subject))
	case names.ControllerTagKind:
		return errors.Trace(m.st.removeControllerUser(subject))
	}
	return errors.NotValidf("%q as a target", target.Kind())
}
