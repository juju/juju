// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

var validUser = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9]*$")

// AddUser adds a user to the state.
func (st *State) AddUser(name, password string) (*User, error) {
	if !validUser.MatchString(name) {
		return nil, fmt.Errorf("invalid user name %q", name)
	}
	u := &User{
		st: st,
		doc: userDoc{
			Name:         name,
			PasswordHash: utils.CompatPasswordHash(password),
		},
	}
	ops := []txn.Op{{
		C:      st.users.Name,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: &u.doc,
	}}
	err := st.runTransaction(ops)
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
	err := st.users.Find(D{{"_id", name}}).One(udoc)
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
	PasswordHash string
}

// Name returns the user name,
func (u *User) Name() string {
	return u.doc.Name
}

// Tag returns the Tag for
// the user ("user-$username")
func (u *User) Tag() string {
	return "user-" + u.doc.Name
}

// SetPassword sets the password associated with the user.
func (u *User) SetPassword(password string) error {
	return u.SetPasswordHash(utils.CompatPasswordHash(password))
}

// SetPasswordHash sets the password to the
// inverse of utils.CompatPasswordHash(pwHash).
// It can be used when we know only the hash
// of the password, but not the clear text.
func (u *User) SetPasswordHash(pwHash string) error {
	ops := []txn.Op{{
		C:      u.st.users.Name,
		Id:     u.Name(),
		Update: D{{"$set", D{{"passwordhash", pwHash}}}},
	}}
	if err := u.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set password of user %q: %v", u.Name(), err)
	}
	u.doc.PasswordHash = pwHash
	return nil
}

// PasswordValid returns whether the given password
// is valid for the user.
func (u *User) PasswordValid(password string) bool {
	// Since these are potentially set by a User, we intentionally use the
	// slower pbkdf2 style hashing. Also, we don't expect to have thousands
	// of Users trying to log in at the same time (which we *do* expect of
	// Unit and Machine agents.)
	return utils.CompatPasswordHash(password) == u.doc.PasswordHash
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
