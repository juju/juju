// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// NOTE: the identities that are being stored in the database here are only
// the local identities, like "admin@local" or "bob@local".  In the  world
// where we have external identity providers hooked up, there are no records
// in the databse for identities that are authenticated elsewhere.

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

	identityCollectionName    = "identities"
	localIdentityProviderName = "local"
)

// AddIdentity adds an identity to the database.
func (st *State) AddIdentity(name, displayName, password, creator string) (*Identity, error) {
	// The name of the identity happens to match the regex we use to confirm user names.
	// Identities do not have tags, so there is no special function for identities. Given
	// the relationships between users and identities it seems reasonable to use the same
	// validation check.
	if !names.IsValidUser(name) {
		return nil, errors.Errorf("invalid identity name %q", name)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}
	identity := &Identity{
		st: st,
		doc: identityDoc{
			Name:         name,
			DisplayName:  displayName,
			PasswordHash: utils.UserPasswordHash(password, salt),
			PasswordSalt: salt,
			CreatedBy:    creator,
			DateCreated:  nowToTheSecond(),
		},
	}
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     name,
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

// String returns "<name>@local" where <name> is the Name of the identity.
func (i *Identity) String() string {
	return fmt.Sprintf("%s@%s", i.Name(), localIdentityProviderName)
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
// The resulting time will be nil if the identity has never logged in.  In the
// normal case, the LastLogin is the last time that the identity connected through
// the API server.
func (i *Identity) LastLogin() *time.Time {
	when := i.doc.LastLogin
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

// UpdateLastLogin sets the LastLogin time of the identity to be now (to the
// nearest second).
func (i *Identity) UpdateLastLogin() error {
	timestamp := nowToTheSecond()
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"lastlogin", timestamp}}}},
	}}
	if err := i.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot update last login timestamp for identity %q", i.Name())
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

// setPasswordHash stores the hash and the salt of the password.
func (i *Identity) setPasswordHash(pwHash string, pwSalt string) error {
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"passwordhash", pwHash}, {"passwordsalt", pwSalt}}}},
	}}
	if err := i.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set password of identity %q", i.Name())
	}
	i.doc.PasswordHash = pwHash
	i.doc.PasswordSalt = pwSalt
	return nil
}

// PasswordValid returns whether the given password is valid for the Identity.
func (i *Identity) PasswordValid(password string) bool {
	// If the Identity is deactivated, no point in carrying on. Since any
	// authentication checks are done very soon after the identity is read
	// from the database, there is a very small timeframe where an identity
	// could be disabled after it has been read but prior to being checked,
	// but in practice, this isn't a problem.
	if i.IsDeactivated() {
		return false
	}

	pwHash := utils.UserPasswordHash(password, i.doc.PasswordSalt)
	return pwHash == i.doc.PasswordHash
}

// Refresh refreshes information about the Identity from the state.
func (i *Identity) Refresh() error {
	var udoc identityDoc
	if err := i.st.getIdentity(i.Name(), &udoc); err != nil {
		return err
	}
	i.doc = udoc
	return nil
}

// Deactivate deactivates the identity.  Deactivated identities cannot log in.
func (i *Identity) Deactivate() error {
	if i.doc.Name == AdminIdentity {
		return errors.Unauthorizedf("cannot deactivate admin identity")
	}
	return errors.Annotatef(i.setDeactivated(true), "cannot deactivate identity %q", i.Name())
}

// Activate reactivates the identity, setting disabled to false.
func (i *Identity) Activate() error {
	return errors.Annotatef(i.setDeactivated(false), "cannot activate identity %q", i.Name())
}

func (i *Identity) setDeactivated(value bool) error {
	ops := []txn.Op{{
		C:      identityCollectionName,
		Id:     i.Name(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"deactivated", value}}}},
	}}
	if err := i.st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			err = fmt.Errorf("identity no longer exists")
		}
		return err
	}
	i.doc.Deactivated = value
	return nil
}

// IsDeactivated returns whether the identity is currently deactiviated.
func (i *Identity) IsDeactivated() bool {
	// Yes, this is a cached value, but in practice the identity object is
	// never held around for a long time.
	return i.doc.Deactivated
}
