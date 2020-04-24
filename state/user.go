// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// NOTE: the users that are being stored in the database here are only
// the local users, like "admin" or "bob".  In the  world
// where we have external user providers hooked up, there are no records
// in the database for users that are authenticated elsewhere.

package state

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/permission"
)

const userGlobalKeyPrefix = "us"

func userGlobalKey(userID string) string {
	return fmt.Sprintf("%s#%s", userGlobalKeyPrefix, userID)
}

func userIDFromGlobalKey(key string) string {
	prefix := userGlobalKeyPrefix + "#"
	return strings.TrimPrefix(key, prefix)
}

func (st *State) checkUserExists(name string) (bool, error) {
	lowercaseName := strings.ToLower(name)

	users, closer := st.db().GetCollection(usersC)
	defer closer()

	var count int
	var err error
	if count, err = users.FindId(lowercaseName).Count(); err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddUser adds a user to the database.
func (st *State) AddUser(name, displayName, password, creator string) (*User, error) {
	return st.addUser(name, displayName, password, creator, nil)
}

// AddUserWithSecretKey adds the user with the specified name, and assigns it
// a randomly generated secret key. This secret key may be used for the user
// and controller to mutually authenticate one another, without without relying
// on TLS certificates.
//
// The new user will not have a password. A password must be set, clearing the
// secret key in the process, before the user can login normally.
func (st *State) AddUserWithSecretKey(name, displayName, creator string) (*User, error) {
	secretKey, err := generateSecretKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.addUser(name, displayName, "", creator, secretKey)
}

func (st *State) addUser(name, displayName, password, creator string, secretKey []byte) (*User, error) {
	if !names.IsValidUserName(name) {
		return nil, errors.Errorf("invalid user name %q", name)
	}
	lowercaseName := strings.ToLower(name)

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
		},
	}

	if password != "" {
		salt, err := utils.RandomSalt()
		if err != nil {
			return nil, err
		}
		user.doc.PasswordHash = utils.UserPasswordHash(password, salt)
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

	err := st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.Errorf("username unavailable")
	}
	if err != nil {
		if errors.IsAlreadyExists(err) {
			err = errors.Errorf("username unavailable")
		}
		return nil, errors.Trace(err)
	}
	return user, nil
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
		}
		ops := []txn.Op{{
			Id:     lowercaseName,
			C:      usersC,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"deleted": true}},
		}}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

func createInitialUserOps(controllerUUID string, user names.UserTag, password, salt string, dateCreated time.Time) []txn.Op {
	lowercaseName := strings.ToLower(user.Name())
	doc := userDoc{
		DocID:        lowercaseName,
		Name:         user.Name(),
		DisplayName:  user.Name(),
		PasswordHash: utils.UserPasswordHash(password, salt),
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
		return nil, DeletedUserError{UserName: user.Name()}
	}
	return user, nil
}

// AllUsers returns a slice of state.User. This includes all active users. If
// includeDeactivated is true it also returns inactive users. At this point it
// never returns deleted users.
func (st *State) AllUsers(includeDeactivated bool) ([]*User, error) {
	var result []*User

	users, closer := st.db().GetCollection(usersC)
	defer closer()

	var query bson.D
	// TODO(redir): Provide option to retrieve deleted users in future PR.
	// e.g. if !includeDelted.
	// Ensure the query checks for users without the deleted attribute, and
	// also that if it does that the value is not true. fwereade wanted to be
	// sure we cannot miss users that previously existed without the deleted
	// attr. Since this will only be in 2.0 that should never happen, but...
	// belt and suspenders.
	query = append(query, bson.DocElem{
		"deleted", bson.D{{"$ne", true}},
	})

	// As above, in the case that a user previously existed and doesn't have a
	// deactivated attribute, we make sure the query checks for the existence
	// of the attribute, and if it exists that it is not true.
	if !includeDeactivated {
		query = append(query, bson.DocElem{
			"deactivated", bson.D{{"$ne", true}},
		})
	}
	iter := users.Find(query).Iter()
	defer iter.Close()

	var doc userDoc
	for iter.Next(&doc) {
		result = append(result, &User{st: st, doc: doc})
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	// Always return a predictable order, sort by Name.
	sort.Sort(userList(result))
	return result, nil
}

// User represents a local user in the database.
type User struct {
	st           *State
	doc          userDoc
	lastLoginDoc userLastLoginDoc
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
}

type userLastLoginDoc struct {
	DocID     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`
	// LastLogin is updated by the apiserver whenever the user
	// connects over the API. This update is not done using mgo.txn
	// so this value could well change underneath a normal transaction
	// and as such, it should NEVER appear in any transaction asserts.
	// It is really informational only as far as everyone except the
	// api server is concerned.
	LastLogin time.Time `bson:"last-login"`
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

// LastLogin returns when this User last connected through the API in UTC.
// The resulting time will be nil if the user has never logged in.  In the
// normal case, the LastLogin is the last time that the user connected through
// the API server.
func (u *User) LastLogin() (time.Time, error) {
	lastLogins, closer := u.st.db().GetRawCollection(userLastLoginC)
	defer closer()

	var lastLogin userLastLoginDoc
	err := lastLogins.FindId(u.doc.DocID).Select(bson.D{{"last-login", 1}}).One(&lastLogin)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = errors.Wrap(err, NeverLoggedInError(u.UserTag().Name()))
		}
		return time.Time{}, errors.Trace(err)
	}

	return lastLogin.LastLogin.UTC(), nil
}

// NeverLoggedInError is used to indicate that a user has never logged in.
type NeverLoggedInError string

// Error returns the error string for a user who has never logged
// in.
func (e NeverLoggedInError) Error() string {
	return `never logged in: "` + string(e) + `"`
}

// IsNeverLoggedInError returns true if err is of type NeverLoggedInError.
func IsNeverLoggedInError(err error) bool {
	_, ok := errors.Cause(err).(NeverLoggedInError)
	return ok
}

// UpdateLastLogin sets the LastLogin time of the user to be now (to the
// nearest second).
func (u *User) UpdateLastLogin() (err error) {
	if err := u.ensureNotDeleted(); err != nil {
		return errors.Annotate(err, "cannot update last login")
	}
	lastLogins, closer := u.st.db().GetCollection(userLastLoginC)
	defer closer()

	lastLoginsW := lastLogins.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastLoginsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastLogin := userLastLoginDoc{
		DocID:     u.doc.DocID,
		ModelUUID: u.st.ModelUUID(),
		LastLogin: u.st.nowToTheSecond(),
	}

	_, err = lastLoginsW.UpsertId(lastLogin.DocID, lastLogin)
	return errors.Trace(err)
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
	salt, err := utils.RandomSalt()
	if err != nil {
		return err
	}
	return u.SetPasswordHash(utils.UserPasswordHash(password, salt), salt)
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
	// read from the database, there is a very small timeframe where an user
	// could be disabled after it has been read but prior to being checked, but
	// in practice, this isn't a problem.
	if u.IsDisabled() || u.IsDeleted() {
		return false
	}
	if u.doc.PasswordSalt != "" {
		return utils.UserPasswordHash(password, u.doc.PasswordSalt) == u.doc.PasswordHash
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

// Enable reactivates the user, setting disabled to false.
func (u *User) Enable() error {
	if err := u.ensureNotDeleted(); err != nil {
		return errors.Annotate(err, "cannot enable")
	}
	return errors.Annotatef(u.setDeactivated(false), "cannot enable user %q", u.Name())
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

// DeletedUserError is used to indicate when an attempt to mutate a deleted
// user is attempted.
type DeletedUserError struct {
	UserName string
}

// Error implements the error interface.
func (e DeletedUserError) Error() string {
	return fmt.Sprintf("user %q is permanently deleted", e.UserName)
}

// ensureNotDeleted refreshes the user to ensure it wasn't deleted since we
// acquired it.
func (u *User) ensureNotDeleted() error {
	if err := u.Refresh(); err != nil {
		return errors.Trace(err)
	}
	if u.doc.Deleted {
		return DeletedUserError{u.Name()}
	}
	return nil
}

// ResetPassword clears the user's password (if there is one),
// and generates a new secret key for the user.
// This must be an active user.
func (u *User) ResetPassword() ([]byte, error) {
	var key []byte
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := u.ensureNotDeleted(); err != nil {
			return nil, errors.Trace(err)
		}
		if u.IsDisabled() {
			return nil, fmt.Errorf("user deactivated")
		}
		var err error
		key, err = generateSecretKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		update := bson.D{
			{
				"$set", bson.D{
					{"secretkey", key},
				},
			},
			{
				"$unset", bson.D{
					{"passwordhash", ""},
					{"passwordsalt", ""},
				},
			},
		}
		lowercaseName := strings.ToLower(u.Name())
		return []txn.Op{{
			C:      usersC,
			Id:     lowercaseName,
			Assert: txn.DocExists,
			Update: update,
		}}, nil
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotatef(err, "cannot reset password for user %q", u.Name())
	}
	u.doc.SecretKey = key
	u.doc.PasswordHash = ""
	u.doc.PasswordSalt = ""
	return key, nil
}

// generateSecretKey generates a random, 32-byte secret key.
func generateSecretKey() ([]byte, error) {
	var secretKey [32]byte
	if _, err := rand.Read(secretKey[:]); err != nil {
		return nil, errors.Trace(err)
	}
	return secretKey[:], nil
}

// userList type is used to provide the methods for sorting.
type userList []*User

func (u userList) Len() int           { return len(u) }
func (u userList) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u userList) Less(i, j int) bool { return u[i].Name() < u[j].Name() }
