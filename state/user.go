package state

import (
	"fmt"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/utils"
)

func (st *State) checkUserExists(name string) (bool, error) {
	var count int
	var err error
	if count, err = st.users.FindId(name).Count(); err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddUser adds a user to the state.
func (st *State) AddUser(name, password string) (*User, error) {
	if !names.IsUser(name) {
		return nil, fmt.Errorf("invalid user name %q", name)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}
	u := &User{
		st: st,
		doc: userDoc{
			Name:         name,
			PasswordHash: utils.UserPasswordHash(password, salt),
			PasswordSalt: salt,
		},
	}
	ops := []txn.Op{{
		C:      st.users.Name,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: &u.doc,
	}}
	err = st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = fmt.Errorf("user already exists")
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// getUser fetches information about the user with the
// given name into the provided userDoc.
func (st *State) getUser(name string, udoc *userDoc) error {
	err := st.users.Find(bson.D{{"_id", name}}).One(udoc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("user %q", name)
	}
	return err
}

// User returns the state user for the given name,
func (st *State) User(name string) (*User, error) {
	u := &User{st: st}
	if err := st.getUser(name, &u.doc); err != nil {
		return nil, err
	}
	return u, nil
}

// User represents a juju client user.
type User struct {
	st  *State
	doc userDoc
}

type userDoc struct {
	Name         string `bson:"_id_"`
	Deactivated  bool   // Removing users means they still exist, but are marked deactivated
	PasswordHash string
	PasswordSalt string
}

// Name returns the user name,
func (u *User) Name() string {
	return u.doc.Name
}

// Tag returns the Tag for
// the user ("user-$username")
func (u *User) Tag() string {
	return names.UserTag(u.doc.Name)
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
		C:      u.st.users.Name,
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
		C:      u.st.users.Name,
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
