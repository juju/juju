// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// NOTE: the users that are being stored in the database here are only
// the local users, like "admin" or "bob" (@local).  In the  world
// where we have external user providers hooked up, there are no records
// in the databse for users that are authenticated elsewhere.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

const (
	localUserProviderName = "local"
)

func (st *State) checkUserExists(name string) (bool, error) {
	users, closer := st.getCollection(usersC)
	defer closer()

	var count int
	var err error
	if count, err = users.FindId(name).Count(); err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddAdminUser adds a user with name 'admin' and the given password to the
// state server. It then adds that user as an environment user with
// username 'admin@local', indicating that the user's provider is 'local' i.e.
// the state server.
func (st *State) AddAdminUser(password string) (*User, error) {
	admin, err := st.AddUser(AdminUser, "", password, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	adminTag := admin.UserTag()
	_, err = st.AddEnvironmentUser(adminTag, adminTag, "")
	if err != nil {
		return nil, errors.Annotate(err, "failed to create admin environment user")
	}
	return admin, nil
}

// AddUser adds a user to the database.
func (st *State) AddUser(name, displayName, password, creator string) (*User, error) {
	if !names.IsValidUserName(name) {
		return nil, errors.Errorf("invalid user name %q", name)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}
	user := &User{
		st: st,
		doc: userDoc{
			Name:         name,
			DisplayName:  displayName,
			PasswordHash: utils.UserPasswordHash(password, salt),
			PasswordSalt: salt,
			CreatedBy:    creator,
			DateCreated:  nowToTheSecond(),
		},
	}
	ops := []txn.Op{{
		C:      usersC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: &user.doc,
	}}
	err = st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.New("user already exists")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return user, nil
}

// getUser fetches information about the user with the
// given name into the provided userDoc.
func (st *State) getUser(name string, udoc *userDoc) error {
	users, closer := st.getCollection(usersC)
	defer closer()

	err := users.Find(bson.D{{"_id", name}}).One(udoc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("user %q", name)
	}
	return err
}

// User returns the state User for the given name,
func (st *State) User(name string) (*User, error) {
	user := &User{st: st}
	if err := st.getUser(name, &user.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return user, nil
}

// User represents a local user in the database.
type User struct {
	st  *State
	doc userDoc
}

type userDoc struct {
	Name        string `bson:"_id"`
	DisplayName string `bson:"displayname"`
	// Removing users means they still exist, but are marked deactivated
	Deactivated  bool       `bson:"deactivated"`
	PasswordHash string     `bson:"passwordhash"`
	PasswordSalt string     `bson:"passwordsalt"`
	CreatedBy    string     `bson:"createdby"`
	DateCreated  time.Time  `bson:"datecreated"`
	LastLogin    *time.Time `bson:"lastlogin"`
}

// String returns "<name>@local" where <name> is the Name of the user.
func (u *User) String() string {
	return fmt.Sprintf("%s@%s", u.Name(), localUserProviderName)
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
	return names.NewUserTag(u.doc.Name)
}

// LastLogin returns when this User last connected through the API in UTC.
// The resulting time will be nil if the user has never logged in.  In the
// normal case, the LastLogin is the last time that the user connected through
// the API server.
func (u *User) LastLogin() *time.Time {
	when := u.doc.LastLogin
	if when == nil {
		return nil
	}
	result := when.UTC()
	return &result
}

// nowToTheSecond returns the current time in UTC to the nearest second.
func nowToTheSecond() time.Time {
	return time.Now().Round(time.Second).UTC()
}

// UpdateLastLogin sets the LastLogin time of the user to be now (to the
// nearest second).
func (u *User) UpdateLastLogin() error {
	timestamp := nowToTheSecond()
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"lastlogin", timestamp}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot update last login timestamp for user %q", u.Name())
	}

	u.doc.LastLogin = &timestamp
	return nil
}

// SetPassword sets the password associated with the User.
func (u *User) SetPassword(password string) error {
	salt, err := utils.RandomSalt()
	if err != nil {
		return err
	}
	return u.SetPasswordHash(utils.UserPasswordHash(password, salt), salt)
}

// SetPasswordHash stores the hash and the salt of the password.
func (u *User) SetPasswordHash(pwHash string, pwSalt string) error {
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"passwordhash", pwHash}, {"passwordsalt", pwSalt}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set password of user %q", u.Name())
	}
	u.doc.PasswordHash = pwHash
	u.doc.PasswordSalt = pwSalt
	return nil
}

// PasswordValid returns whether the given password is valid for the User.
func (u *User) PasswordValid(password string) bool {
	// If the User is deactivated, no point in carrying on. Since any
	// authentication checks are done very soon after the user is read
	// from the database, there is a very small timeframe where an user
	// could be disabled after it has been read but prior to being checked,
	// but in practice, this isn't a problem.
	if u.IsDeactivated() {
		return false
	}
	if u.doc.PasswordSalt != "" {
		return utils.UserPasswordHash(password, u.doc.PasswordSalt) == u.doc.PasswordHash
	}
	// In Juju 1.16 and older, we did not set a Salt for the user password,
	// so check if the password hash matches using CompatSalt. if it
	// does, then set the password again so that we get a proper salt
	if utils.UserPasswordHash(password, utils.CompatSalt) == u.doc.PasswordHash {
		// This will set a new Salt for the password. We ignore if it
		// fails because we will try again at the next request
		logger.Debugf("User %s logged in with CompatSalt resetting password for new salt",
			u.Name())
		err := u.SetPassword(password)
		if err != nil {
			logger.Errorf("Cannot set resalted password for user %q", u.Name())
		}
		return true
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

// Deactivate deactivates the user.  Deactivated identities cannot log in.
func (u *User) Deactivate() error {
	if u.doc.Name == AdminUser {
		return errors.Unauthorizedf("cannot deactivate admin user")
	}
	return errors.Annotatef(u.setDeactivated(true), "cannot deactivate user %q", u.Name())
}

// Activate reactivates the user, setting disabled to false.
func (u *User) Activate() error {
	return errors.Annotatef(u.setDeactivated(false), "cannot activate user %q", u.Name())
}

func (u *User) setDeactivated(value bool) error {
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"deactivated", value}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = fmt.Errorf("user no longer exists")
		}
		return err
	}
	u.doc.Deactivated = value
	return nil
}

// IsDeactivated returns whether the user is currently deactiviated.
func (u *User) IsDeactivated() bool {
	// Yes, this is a cached value, but in practice the user object is
	// never held around for a long time.
	return u.doc.Deactivated
}
