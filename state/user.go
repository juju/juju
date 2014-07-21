package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
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

func (st *State) AddAdminUser(password string) (*User, error) {
	return st.AddUser("admin", "", password, "")
}

// AddUser adds a user to the state.
func (st *State) AddUser(username, displayName, password, creator string) (*User, error) {
	if !names.IsUser(username) {
		return nil, errors.Errorf("invalid user name %q", username)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().Round(time.Second).UTC()
	u := &User{
		st: st,
		doc: userDoc{
			Name:         username,
			DisplayName:  displayName,
			PasswordHash: utils.UserPasswordHash(password, salt),
			PasswordSalt: salt,
			CreatedBy:    creator,
			DateCreated:  timestamp,
		},
	}
	ops := []txn.Op{{
		C:      usersC,
		Id:     username,
		Assert: txn.DocMissing,
		Insert: &u.doc,
	}}
	err = st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.New("user already exists")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
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

// User returns the state user for the given name,
func (st *State) User(name string) (*User, error) {
	u := &User{st: st}
	if err := st.getUser(name, &u.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// User represents a juju client user.
type User struct {
	st  *State
	doc userDoc
}

type userDoc struct {
	Name           string `bson:"_id_"`
	DisplayName    string
	Deactivated    bool // Removing users means they still exist, but are marked deactivated
	PasswordHash   string
	PasswordSalt   string
	CreatedBy      string
	DateCreated    time.Time
	LastConnection time.Time
}

// Name returns the user name,
func (u *User) Name() string {
	return u.doc.Name
}

// DisplayName returns the display name of the user.
func (u *User) DisplayName() string {
	return u.doc.DisplayName
}

// CreatedBy returns the name of the user that created this user.
func (u *User) CreatedBy() string {
	return u.doc.CreatedBy
}

// DateCreated returns when this user was created in UTC.
func (u *User) DateCreated() time.Time {
	return u.doc.DateCreated.UTC()
}

// LastConnection returns when this user last connected through the API in UTC.
func (u *User) LastConnection() *time.Time {
	result := u.doc.LastConnection
	if result.IsZero() {
		return nil
	}

	result = result.UTC()
	return &result
}

func (u *User) UpdateLastConnection() error {
	timestamp := time.Now().Round(time.Second).UTC()

	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Update: bson.D{{"$set", bson.D{{"lastconnection", timestamp}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot update last connection timestamp for user %q", u.Name())
	}

	u.doc.LastConnection = timestamp
	return nil
}

// Tag returns the Tag for the User.
func (u *User) Tag() names.Tag {
	return names.NewUserTag(u.doc.Name)
}

// SetPassword sets the password associated with the user.
func (u *User) SetPassword(password string) error {
	salt, err := utils.RandomSalt()
	if err != nil {
		return err
	}
	return u.SetPasswordHash(utils.UserPasswordHash(password, salt), salt)
}

// SetPasswordHash sets the password to the
// inverse of pwHash = utils.UserPasswordHash(pw, pwSalt).
// It can be used when we know only the hash
// of the password, but not the clear text.
func (u *User) SetPasswordHash(pwHash string, pwSalt string) error {
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Update: bson.D{{"$set", bson.D{{"passwordhash", pwHash}, {"passwordsalt", pwSalt}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set password of user %q: %v", u.Name(), err)
	}
	u.doc.PasswordHash = pwHash
	u.doc.PasswordSalt = pwSalt
	return nil
}

// PasswordValid returns whether the given password
// is valid for the user.
func (u *User) PasswordValid(password string) bool {
	// If the user is deactivated, no point in carrying on
	if u.IsDeactivated() {
		return false
	}
	// Since these are potentially set by a User, we intentionally use the
	// slower pbkdf2 style hashing. Also, we don't expect to have thousands
	// of Users trying to log in at the same time (which we *do* expect of
	// Unit and Machine agents.)
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

// Refresh refreshes information about the user
// from the state.
func (u *User) Refresh() error {
	var udoc userDoc
	if err := u.st.getUser(u.Name(), &udoc); err != nil {
		return err
	}
	u.doc = udoc
	return nil
}

func (u *User) Deactivate() error {
	if u.doc.Name == AdminUser {
		return errors.Unauthorizedf("Can't deactivate admin user")
	}
	ops := []txn.Op{{
		C:      usersC,
		Id:     u.Name(),
		Update: bson.D{{"$set", bson.D{{"deactivated", true}}}},
		Assert: txn.DocExists,
	}}
	if err := u.st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = fmt.Errorf("user no longer exists")
		}
		return fmt.Errorf("cannot deactivate user %q: %v", u.Name(), err)
	}
	u.doc.Deactivated = true
	return nil
}

func (u *User) IsDeactivated() bool {
	return u.doc.Deactivated
}
