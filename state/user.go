// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// NOTE: the users that are being stored in the database here are only
// the local users, like "admin" or "bob" (@local).  In the  world
// where we have external user providers hooked up, there are no records
// in the databse for users that are authenticated elsewhere.

package state

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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
	// Generate a random, 32-byte secret key. This can be used
	// to obtain the controller's (self-signed) CA certificate
	// and set the user's password.
	var secretKey [32]byte
	if _, err := rand.Read(secretKey[:]); err != nil {
		return nil, errors.Trace(err)
	}
	return st.addUser(name, displayName, "", creator, secretKey[:])
}

func (st *State) addUser(name, displayName, password, creator string, secretKey []byte) (*User, error) {
	if !names.IsValidUserName(name) {
		return nil, errors.Errorf("invalid user name %q", name)
	}
	nameToLower := strings.ToLower(name)

	user := &User{
		st: st,
		doc: userDoc{
			DocID:       nameToLower,
			Name:        name,
			DisplayName: displayName,
			SecretKey:   secretKey,
			CreatedBy:   creator,
			DateCreated: nowToTheSecond(),
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
		Id:     nameToLower,
		Assert: txn.DocMissing,
		Insert: &user.doc,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("user")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return user, nil
}

// RemoveUser removes the user with the given name from the User collection as
// well as any modeluser entries in the ModelUser collection.
func (st *State) RemoveUser(user names.UserTag) error {

	name := strings.ToLower(user.Name())

	_, err := st.User(user)
	if errors.IsNotFound(err) {
		return errors.NewUserNotFound(err, "user not found")
	}
	// This builds the transaction operations we want to try for the User.
	buildTxn := func(attempt int) ([]txn.Op, error) {

		ops := []txn.Op{{
			Id:     name,
			C:      usersC,
			Assert: txn.DocExists,
			Remove: true,
		}}
		return ops, nil
	}
	err = st.run(buildTxn)

	// If there is no User then we return with a user not found error.
	if err == jujutxn.ErrNoOperations {
		return errors.NewUserNotFound(err, "user not found")
	}

	// Get a raw connection to the ModelUser collection.
	modelUsers, userCloser := st.getRawCollection(modelUsersC)
	defer userCloser()

	// Build the transaction for removing all ModelUsers for the user.
	buildTxn = func(attempt int) ([]txn.Op, error) {
		var modelUserIDs []struct {
			ID string `bson:"_id"`
		}
		// The `_id` is munged in the ModelUser collection because it is a
		// *local* collection. Here we pierce the veil in the interest of speed
		// and simplicity. Rather than building up the `_id` from the name and
		// model UUID in a series of loops we simply return the _ids which we
		// need to remove. This was left inside this function instead of
		// creating an external helper to prevent confusion with the existing
		// RemoveModelUser function which has a different purpose: namely
		// unsharing a model.
		err := modelUsers.Find(bson.D{
			{"user", user.Canonical()}}).Select(bson.D{{"_id", 1}}).All(&modelUserIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		for _, entry := range modelUserIDs {
			ops = append(ops, txn.Op{
				C:      modelUsersC,
				Id:     entry.ID,
				Assert: txn.DocExists,
				Remove: true,
			})
		}
		return ops, nil
	}
	return st.run(buildTxn)
}

func createInitialUserOp(st *State, user names.UserTag, password, salt string) txn.Op {
	nameToLower := strings.ToLower(user.Name())
	doc := userDoc{
		DocID:        nameToLower,
		Name:         user.Name(),
		DisplayName:  user.Name(),
		PasswordHash: utils.UserPasswordHash(password, salt),
		PasswordSalt: salt,
		CreatedBy:    user.Name(),
		DateCreated:  nowToTheSecond(),
	}
	return txn.Op{
		C:      usersC,
		Id:     nameToLower,
		Assert: txn.DocMissing,
		Insert: &doc,
	}
}

// getUser fetches information about the user with the
// given name into the provided userDoc.
func (st *State) getUser(name string, udoc *userDoc) error {
	users, closer := st.getCollection(usersC)
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
		return nil, errors.NotFoundf("user %q", tag.Canonical())
	}
	user := &User{st: st}
	if err := st.getUser(tag.Name(), &user.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return user, nil
}

// User returns the state User for the given name,
func (st *State) AllUsers(includeDeactivated bool) ([]*User, error) {
	var result []*User

	users, closer := st.getCollection(usersC)
	defer closer()

	var query bson.D
	if !includeDeactivated {
		query = append(query, bson.DocElem{"deactivated", false})
	}
	iter := users.Find(query).Iter()
	defer iter.Close()

	var doc userDoc
	for iter.Next(&doc) {
		result = append(result, &User{st: st, doc: doc})
	}
	if err := iter.Err(); err != nil {
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
	DocID       string `bson:"_id"`
	Name        string `bson:"name"`
	DisplayName string `bson:"displayname"`
	// Removing users means they still exist, but are marked deactivated
	Deactivated  bool      `bson:"deactivated"`
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

// String returns "<name>@local" where <name> is the Name of the user.
func (u *User) String() string {
	return u.UserTag().Canonical()
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
	if name == "" {
		// TODO(waigani) This is a hack for upgrades to 1.23. Once we are no
		// longer tied to 1.23, we can confidently always use u.doc.Name.
		name = u.doc.DocID
	}
	return names.NewLocalUserTag(name)
}

// LastLogin returns when this User last connected through the API in UTC.
// The resulting time will be nil if the user has never logged in.  In the
// normal case, the LastLogin is the last time that the user connected through
// the API server.
func (u *User) LastLogin() (time.Time, error) {
	lastLogins, closer := u.st.getRawCollection(userLastLoginC)
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

// nowToTheSecond returns the current time in UTC to the nearest second.
// We use this for a time source that is not more precise than we can
// handle. When serializing time in and out of mongo, we lose enough
// precision that it's misleading to store any more than precision to
// the second.
// TODO(fwereade): 2016-03-17 lp:1558657
var nowToTheSecond = func() time.Time { return time.Now().Round(time.Second).UTC() }

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
	lastLogins, closer := u.st.getCollection(userLastLoginC)
	defer closer()

	lastLoginsW := lastLogins.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastLoginsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastLogin := userLastLoginDoc{
		DocID:     u.doc.DocID,
		ModelUUID: u.st.ModelUUID(),
		LastLogin: nowToTheSecond(),
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
	update := bson.D{{"$set", bson.D{
		{"passwordhash", pwHash},
		{"passwordsalt", pwSalt},
	}}}
	if u.doc.SecretKey != nil {
		update = append(update,
			bson.DocElem{"$unset", bson.D{{"secretkey", ""}}},
		)
	}
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Assert: txn.DocExists,
		Update: update,
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set password of user %q", u.Name())
	}
	u.doc.PasswordHash = pwHash
	u.doc.PasswordSalt = pwSalt
	u.doc.SecretKey = nil
	return nil
}

// PasswordValid returns whether the given password is valid for the User.
func (u *User) PasswordValid(password string) bool {
	// If the User is deactivated, no point in carrying on. Since any
	// authentication checks are done very soon after the user is read
	// from the database, there is a very small timeframe where an user
	// could be disabled after it has been read but prior to being checked,
	// but in practice, this isn't a problem.
	if u.IsDisabled() {
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
	environment, err := u.st.ControllerModel()
	if err != nil {
		return errors.Trace(err)
	}
	if u.doc.Name == environment.Owner().Name() {
		return errors.Unauthorizedf("cannot disable controller model owner")
	}
	return errors.Annotatef(u.setDeactivated(true), "cannot disable user %q", u.Name())
}

// Enable reactivates the user, setting disabled to false.
func (u *User) Enable() error {
	return errors.Annotatef(u.setDeactivated(false), "cannot enable user %q", u.Name())
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

// IsDisabled returns whether the user is currently enabled.
func (u *User) IsDisabled() bool {
	// Yes, this is a cached value, but in practice the user object is
	// never held around for a long time.
	return u.doc.Deactivated
}

// userList type is used to provide the methods for sorting.
type userList []*User

func (u userList) Len() int           { return len(u) }
func (u userList) Swap(i, j int)      { u[i], u[j] = u[j], u[i] }
func (u userList) Less(i, j int) bool { return u[i].Name() < u[j].Name() }
