// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// NOTE: the users that are being stored in the database here are only
// the local users, like "admin" or "bob".  In the  world
// where we have external user providers hooked up, there are no records
// in the database for users that are authenticated elsewhere.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/permission"
	internalpassword "github.com/juju/juju/internal/password"
)

const userGlobalKeyPrefix = "us"

func userGlobalKey(userID string) string {
	return fmt.Sprintf("%s#%s", userGlobalKeyPrefix, userID)
}

func userIDFromGlobalKey(key string) string {
	prefix := userGlobalKeyPrefix + "#"
	return strings.TrimPrefix(key, prefix)
}

// AddUser adds a user to the database.
func (st *State) AddUser(name, displayName, password, creator string) (*User, error) {
	return st.addUser(name, displayName, password, creator, nil)
}

// AddUserWithSecretKey adds the user with the specified name, and assigns it
// a randomly generated secret key. This secret key may be used for the user
// and controller to mutually authenticate one another, without relying
// on TLS certificates.
//
// The new user will not have a password. A password must be set, clearing the
// secret key in the process, before the user can login normally.
func (st *State) AddUserWithSecretKey(name, displayName, creator string) (*User, error) {
	return st.addUser(name, displayName, "", creator, []byte("big-secret"))
}

func (st *State) addUser(name, displayName, password, creator string, secretKey []byte) (*User, error) {

	if !names.IsValidUserName(name) {
		return nil, errors.Errorf("invalid user name %q", name)
	}
	lowercaseName := strings.ToLower(name)

	foundUser := &User{st: st}
	err := st.getUser(names.NewUserTag(name).Name(), &foundUser.doc)
	// No error, the user is already there
	if err == nil {
		if foundUser.doc.Deleted {
			// the user was deleted, we update it
			return st.recreateExistingUser(foundUser, name, displayName, password, creator, secretKey)
		} else {
			return nil, errors.AlreadyExistsf("user %s", name)
		}
	}

	// There is an error different from not found
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	dateCreated := st.nowToTheSecond()
	user := &User{
		st: st,
		doc: userDoc{
			DocID:       lowercaseName,
			Name:        name,
			DisplayName: displayName,
			SecretKey:   secretKey,
			CreatedBy:   creator,
			DateCreated: dateCreated,
			Deleted:     false,
			RemovalLog:  []userRemovedLogEntry{},
		},
	}

	if password != "" {
		salt, err := internalpassword.RandomSalt()
		if err != nil {
			return nil, err
		}
		user.doc.PasswordHash = internalpassword.UserPasswordHash(password, salt)
		user.doc.PasswordSalt = salt
	}

	ops := []txn.Op{{
		C:      usersC,
		Id:     lowercaseName,
		Assert: txn.DocMissing,
		Insert: &user.doc,
	}}
	controllerUserOps := createControllerUserOps(st.ControllerUUID(),
		names.NewUserTag(name),
		names.NewUserTag(creator),
		displayName,
		dateCreated,
		defaultControllerPermission)
	ops = append(ops, controllerUserOps...)

	err = st.db().RunTransaction(ops)
	if err != nil {
		if err == txn.ErrAborted {
			err = errors.Errorf("username unavailable")
		}
		return nil, errors.Trace(err)
	}
	return user, nil
}

func (st *State) recreateExistingUser(u *User, name, displayName, password, creator string, secretKey []byte) (*User, error) {
	dateCreated := st.nowToTheSecond()
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := u.Refresh()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !u.IsDeleted() {
				return nil, errors.AlreadyExistsf("user %s", name)
			}
		}

		updateUser := bson.D{{"$set", bson.D{
			{"deleted", false},
			{"name", name},
			{"displayname", displayName},
			{"createdby", creator},
			{"datecreated", dateCreated},
			{"secretkey", secretKey},
		}}}

		// update the password
		if password != "" {
			salt, err := internalpassword.RandomSalt()
			if err != nil {
				return nil, err
			}
			updateUser = append(updateUser,
				bson.DocElem{"$set", bson.D{
					{"passwordhash", internalpassword.UserPasswordHash(password, salt)},
					{"passwordsalt", salt},
				}},
			)
		}

		var ops []txn.Op

		// ensure models that were migrating at the time of the RemoveUser call are
		// processed now.
		modelQuery, closer, err := st.modelQueryForUser(u.UserTag(), false)
		defer closer()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var modelDocs []modelDoc
		if err := modelQuery.All(&modelDocs); err != nil {
			return nil, errors.Trace(err)
		}
		for _, model := range modelDocs {
			// remove the permission for the model
			ops = append(ops, removeModelUserOpsGlobal(model.UUID, u.UserTag())...)
		}

		// remove previous controller permissions
		if _, err := u.st.controllerUser(u.UserTag()); err == nil {
			ops = append(ops, removeControllerUserOps(st.ControllerUUID(), u.UserTag())...)
		} else if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}

		// create default new ones
		ops = append(ops, createControllerUserOps(st.ControllerUUID(),
			u.UserTag(),
			names.NewUserTag(creator),
			displayName,
			dateCreated,
			defaultControllerPermission)...)

		// update user doc
		ops = append(ops, txn.Op{
			C:  usersC,
			Id: strings.ToLower(u.Name()),
			Assert: bson.M{
				"deleted": true,
			},
			Update: updateUser,
		})

		return ops, nil
	}

	if err := u.st.db().RunRaw(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}

	// recreate the user object
	return st.User(u.UserTag())
}

// RemoveUser marks the user as deleted. This obviates the ability of a user
// to function, but keeps the userDoc retaining provenance, i.e. auditing.
func (st *State) RemoveUser(tag names.UserTag) error {
	lowercaseName := strings.ToLower(tag.Name())

	u, err := st.User(tag)
	if err != nil {
		return errors.Trace(err)
	}
	if u.IsDeleted() {
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// If it is not our first attempt, refresh the user.
			if err := u.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			if u.IsDeleted() {
				return nil, nil
			}
		}

		// remove the access to all the models and the current controller
		// first query all the models for this user
		modelQuery, closer, err := st.modelQueryForUser(tag, false)
		defer closer()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var modelDocs []modelDoc
		if err := modelQuery.All(&modelDocs); err != nil {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		for _, model := range modelDocs {
			// remove the permission for the model
			ops = append(ops, removeModelUserOpsGlobal(model.UUID, tag)...)
		}

		// remove the user from the controller
		ops = append(ops, removeControllerUserOps(st.ControllerUUID(), tag)...)

		// new entry in the removal log
		newRemovalLogEntry := userRemovedLogEntry{
			RemovedBy:   u.doc.CreatedBy,
			DateCreated: u.doc.DateCreated,
			DateRemoved: st.nowToTheSecond(),
		}
		ops = append(ops, txn.Op{
			Id:     lowercaseName,
			C:      usersC,
			Assert: txn.DocExists,
			Update: bson.M{
				"$set": bson.M{
					"deleted": true,
				},
				"$push": bson.M{
					"removallog": bson.M{"$each": []userRemovedLogEntry{newRemovalLogEntry}},
				},
			},
		})
		return ops, nil
	}

	// Use raw transactions to avoid model filtering
	return st.db().RunRaw(buildTxn)
}

func createInitialUserOps(controllerUUID string, user names.UserTag, password, salt string, dateCreated time.Time) []txn.Op {
	lowercaseName := strings.ToLower(user.Name())
	doc := userDoc{
		DocID:        lowercaseName,
		Name:         user.Name(),
		DisplayName:  user.Name(),
		PasswordHash: internalpassword.UserPasswordHash(password, salt),
		PasswordSalt: salt,
		CreatedBy:    user.Name(),
		DateCreated:  dateCreated,
	}
	ops := []txn.Op{{
		C:      usersC,
		Id:     lowercaseName,
		Assert: txn.DocMissing,
		Insert: &doc,
	}}
	controllerUserOps := createControllerUserOps(controllerUUID,
		names.NewUserTag(user.Name()),
		names.NewUserTag(user.Name()),
		user.Name(),
		dateCreated,
		// first user is controller admin.
		permission.SuperuserAccess)

	ops = append(ops, controllerUserOps...)
	return ops
}

// getUser fetches information about the user with the
// given name into the provided userDoc.
func (st *State) getUser(name string, udoc *userDoc) error {
	users, closer := st.db().GetCollection(usersC)
	defer closer()

	name = strings.ToLower(name)
	err := users.Find(bson.D{{"_id", name}}).One(udoc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("user %q", name)
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	udoc.DateCreated = udoc.DateCreated.UTC()
	return err
}

// User returns the state User for the given name.
func (st *State) User(tag names.UserTag) (*User, error) {
	if !tag.IsLocal() {
		return nil, errors.NotFoundf("user %q", tag.Id())
	}
	user := &User{st: st}
	if err := st.getUser(tag.Name(), &user.doc); err != nil {
		return nil, errors.Trace(err)
	}
	if user.doc.Deleted {
		// This error is returned to the apiserver and from there to the api
		// client. So we don't annotate with information regarding deletion.
		// TODO(redir): We'll return a deletedUserError in the future so we can
		// return more appropriate errors, e.g. username not available.
		return nil, newDeletedUserError(user.Name())
	}
	return user, nil
}

// User represents a local user in the database.
type User struct {
	st  *State
	doc userDoc
}

type userDoc struct {
	DocID        string    `bson:"_id"`
	Name         string    `bson:"name"`
	DisplayName  string    `bson:"displayname"`
	Deactivated  bool      `bson:"deactivated,omitempty"`
	Deleted      bool      `bson:"deleted,omitempty"` // Deleted users are marked deleted but not removed.
	SecretKey    []byte    `bson:"secretkey,omitempty"`
	PasswordHash string    `bson:"passwordhash"`
	PasswordSalt string    `bson:"passwordsalt"`
	CreatedBy    string    `bson:"createdby"`
	DateCreated  time.Time `bson:"datecreated"`
	// RemovalLog keeps a track of removals for this user
	RemovalLog []userRemovedLogEntry `bson:"removallog"`
}

// userRemovedLog contains a log of entries added every time the user
// doc has been removed
type userRemovedLogEntry struct {
	RemovedBy   string    `bson:"removedby"`
	DateCreated time.Time `bson:"datecreated"`
	DateRemoved time.Time `bson:"dateremoved"`
}

// String returns "<name>" where <name> is the Name of the user.
func (u *User) String() string {
	return u.UserTag().Id()
}

// Name returns the User name.
func (u *User) Name() string {
	return u.doc.Name
}

// DisplayName returns the display name of the User.
func (u *User) DisplayName() string {
	return u.doc.DisplayName
}

// CreatedBy returns the name of the User that created this User.
func (u *User) CreatedBy() string {
	return u.doc.CreatedBy
}

// DateCreated returns when this User was created in UTC.
func (u *User) DateCreated() time.Time {
	return u.doc.DateCreated.UTC()
}

// Tag returns the Tag for the User.
func (u *User) Tag() names.Tag {
	return u.UserTag()
}

// UserTag returns the Tag for the User.
func (u *User) UserTag() names.UserTag {
	name := u.doc.Name
	return names.NewLocalUserTag(name)
}

// SecretKey returns the user's secret key, if any.
func (u *User) SecretKey() []byte {
	return u.doc.SecretKey
}

// SetPassword sets the password associated with the User.
func (u *User) SetPassword(password string) error {
	if err := u.ensureNotDeleted(); err != nil {
		return errors.Annotate(err, "cannot set password")
	}
	salt, err := internalpassword.RandomSalt()
	if err != nil {
		return err
	}
	return u.SetPasswordHash(internalpassword.UserPasswordHash(password, salt), salt)
}

// SetPasswordHash stores the hash and the salt of the
// password. If the User has a secret key set then it
// will be cleared.
func (u *User) SetPasswordHash(pwHash string, pwSalt string) error {
	if err := u.ensureNotDeleted(); err != nil {
		// If we do get a late set of the password this is fine b/c we have an
		// explicit check before login.
		return errors.Annotate(err, "cannot set password hash")
	}
	update := bson.D{{"$set", bson.D{
		{"passwordhash", pwHash},
		{"passwordsalt", pwSalt},
	}}}
	if u.doc.SecretKey != nil {
		update = append(update,
			bson.DocElem{"$unset", bson.D{{"secretkey", ""}}},
		)
	}
	lowercaseName := strings.ToLower(u.Name())
	ops := []txn.Op{{
		C:      usersC,
		Id:     lowercaseName,
		Assert: txn.DocExists,
		Update: update,
	}}
	if err := u.st.db().RunTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set password of user %q", u.Name())
	}
	u.doc.PasswordHash = pwHash
	u.doc.PasswordSalt = pwSalt
	u.doc.SecretKey = nil
	return nil
}

// PasswordValid returns whether the given password is valid for the User. The
// caller should call user.Refresh before calling this.
func (u *User) PasswordValid(password string) bool {
	// If the User is deactivated or deleted, there is no point in carrying on.
	// Since any authentication checks are done very soon after the user is
	// read from the database, there is a very small timeframe where a user
	// could be disabled after it has been read but prior to being checked, but
	// in practice, this isn't a problem.
	if u.IsDisabled() || u.IsDeleted() {
		return false
	}
	if u.doc.PasswordSalt != "" {
		return internalpassword.UserPasswordHash(password, u.doc.PasswordSalt) == u.doc.PasswordHash
	}
	return false
}

// Refresh refreshes information about the User from the state.
func (u *User) Refresh() error {
	var udoc userDoc
	if err := u.st.getUser(u.Name(), &udoc); err != nil {
		return err
	}
	u.doc = udoc
	return nil
}

// Disable deactivates the user.  Disabled identities cannot log in.
func (u *User) Disable() error {
	if err := u.ensureNotDeleted(); err != nil {
		return errors.Annotate(err, "cannot disable")
	}
	owner, err := u.st.ControllerOwner()
	if err != nil {
		return errors.Trace(err)
	}
	if u.doc.Name == owner.Name() {
		return errors.Unauthorizedf("cannot disable controller model owner")
	}
	return errors.Annotatef(u.setDeactivated(true), "cannot disable user %q", u.Name())
}

func (u *User) setDeactivated(value bool) error {
	lowercaseName := strings.ToLower(u.Name())
	ops := []txn.Op{{
		C:      usersC,
		Id:     lowercaseName,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"deactivated", value}}}},
	}}
	if err := u.st.db().RunTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = fmt.Errorf("user no longer exists")
		}
		return err
	}
	u.doc.Deactivated = value
	return nil
}

// IsDisabled returns whether the user is currently enabled.
func (u *User) IsDisabled() bool {
	// Yes, this is a cached value, but in practice the user object is
	// never held around for a long time.
	return u.doc.Deactivated
}

// IsDeleted returns whether the user is currently deleted.
func (u *User) IsDeleted() bool {
	return u.doc.Deleted
}

// ensureNotDeleted refreshes the user to ensure it wasn't deleted since we
// acquired it.
func (u *User) ensureNotDeleted() error {
	if err := u.Refresh(); err != nil {
		return errors.Trace(err)
	}
	if u.doc.Deleted {
		return newDeletedUserError(u.Name())
	}
	return nil
}
