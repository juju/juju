// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

const (
	// AdminIdentity is the name of the identity that is created during bootstrap time,
	// and associated with the admin user in the initial environment.
	AdminIdentity = "admin"

	identityCollectionName = "identities"
)

// AddIdentity adds an identity to the database.
func (st *State) AddIdentity(username, displayName, password, creator string) (*Identity, error) {
	if !names.IsValidUser(username) {
		return nil, errors.Errorf("invalid identity name %q", username)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().Round(time.Second).UTC()
	identity := &Identity{
		st: st,
		doc: identityDoc{
			Name:         username,
			DisplayName:  displayName,
			PasswordHash: utils.UserPasswordHash(password, salt),
			PasswordSalt: salt,
			CreatedBy:    creator,
			DateCreated:  timestamp,
		},
	}
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     username,
		Assert: txn.DocMissing,
		Insert: &identity.doc,
	}}
	err = st.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.New("identity already exists")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return identity, nil
}

// getIdentity fetches information about the Identity with the
// given name into the provided identityDoc.
func (st *State) getIdentity(name string, doc *identityDoc) error {
	err := st.db.C(identityCollectionName).Find(bson.D{{"_id", name}}).One(doc)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("identity %q", name)
	}
	return err
}

// Identity returns the state Identity for the given name,
func (st *State) Identity(name string) (*Identity, error) {
	identity := &Identity{st: st}
	if err := st.getIdentity(name, &identity.doc); err != nil {
		return nil, errors.Trace(err)
	}
	return identity, nil
}

// Identity represents a local identity in the database.
type Identity struct {
	st  *State
	doc identityDoc
}

type identityDoc struct {
	Name         string     `bson:"_id"`
	DisplayName  string     `bson:"displayname"`
	Deactivated  bool       `bson:"deactivated"`
	PasswordHash string     `bson:"passwordhash"`
	PasswordSalt string     `bson:"passwordsalt"`
	CreatedBy    string     `bson:"createdby"`
	DateCreated  time.Time  `bson:"datecreated"`
	LastLogin    *time.Time `bson:"lastlogin"`
}

// Name returns the Identity name.
func (i *Identity) Name() string {
	return i.doc.Name
}

// DisplayName returns the display name of the Identity.
func (i *Identity) DisplayName() string {
	return i.doc.DisplayName
}

// CreatedBy returns the name of the Identity that created this Identity.
func (i *Identity) CreatedBy() string {
	return i.doc.CreatedBy
}

// DateCreated returns when this Identity was created in UTC.
func (i *Identity) DateCreated() time.Time {
	return i.doc.DateCreated.UTC()
}

// LastLogin returns when this Identity last connected through the API in UTC.
func (i *Identity) LastLogin() *time.Time {
	when := i.doc.LastLogin
	if when == nil {
		return nil
	}
	result := when.UTC()
	return &result
}

func (i *Identity) UpdateLastLogin() error {
	timestamp := time.Now().Round(time.Second).UTC()

	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Update: bson.D{{"$set", bson.D{{"lastlogin", timestamp}}}},
	}}
	if err := i.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot update last login timestamp for Identity %q", i.Name())
	}

	i.doc.LastLogin = &timestamp
	return nil
}

// SetPassword sets the password associated with the Identity.
func (i *Identity) SetPassword(password string) error {
	salt, err := utils.RandomSalt()
	if err != nil {
		return err
	}
	return i.setPasswordHash(utils.UserPasswordHash(password, salt), salt)
}

// setPasswordHash just stores the hash and the salt of the password.
func (i *Identity) setPasswordHash(pwHash string, pwSalt string) error {
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Update: bson.D{{"$set", bson.D{{"passwordhash", pwHash}, {"passwordsalt", pwSalt}}}},
	}}
	if err := i.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set password of Identity %q: %v", i.Name(), err)
	}
	i.doc.PasswordHash = pwHash
	i.doc.PasswordSalt = pwSalt
	return nil
}

// PasswordValid returns whether the given password
// is valid for the Identity.
func (i *Identity) PasswordValid(password string) bool {
	// If the Identity is deactivated, no point in carrying on.
	if i.IsDeactivated() {
		return false
	}
	return utils.UserPasswordHash(password, i.doc.PasswordSalt) == i.doc.PasswordHash
}

// Refresh refreshes information about the Identity
// from the state.
func (i *Identity) Refresh() error {
	var udoc identityDoc
	if err := i.st.getIdentity(i.Name(), &udoc); err != nil {
		return err
	}
	i.doc = udoc
	return nil
}

func (i *Identity) Deactivate() error {
	if i.doc.Name == AdminIdentity {
		return errors.Unauthorizedf("can't deactivate admin identity")
	}
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Update: bson.D{{"$set", bson.D{{"deactivated", true}}}},
		Assert: txn.DocExists,
	}}
	if err := i.st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = fmt.Errorf("Identity no longer exists")
		}
		return fmt.Errorf("cannot deactivate Identity %q: %v", i.Name(), err)
	}
	i.doc.Deactivated = true
	return nil
}

func (i *Identity) IsDeactivated() bool {
	return i.doc.Deactivated
}
