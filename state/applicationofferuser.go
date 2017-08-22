// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/permission"
)

// GetOfferAccess gets the access permission for the specified user on an offer.
func (st *State) GetOfferAccess(offerUUID string, user names.UserTag) (permission.Access, error) {
	m, err := st.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	perm, err := m.userPermission(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)))
	if err != nil {
		return "", errors.Trace(err)
	}
	return perm.access(), nil
}

// CreateOfferAccess creates a new access permission for a user on an offer.
func (st *State) CreateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if err := permission.ValidateOfferAccess(access); err != nil {
		return errors.Trace(err)
	}

	// Local users must exist.
	if user.IsLocal() {
		_, err := st.User(user)
		if err != nil {
			if errors.IsNotFound(err) {
				return errors.Annotatef(err, "user %q does not exist locally", user.Name())
			}
			return errors.Trace(err)
		}
	}

	offerUUID, err := applicationOfferUUID(st, offer.Name)
	if err != nil {
		return errors.Annotate(err, "creating offer access")
	}
	op := createPermissionOp(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)), access)

	err = st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.AlreadyExistsf("permission for user %q for offer %q", user.Id(), offer.Name)
	}
	return errors.Trace(err)
}

// UpdateOfferAccess changes the user's access permissions on an offer.
func (st *State) UpdateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if err := permission.ValidateOfferAccess(access); err != nil {
		return errors.Trace(err)
	}
	offerUUID, err := applicationOfferUUID(st, offer.Name)
	if err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)), access)

	err = st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// RemoveOfferAccess removes the access permission for a user on an offer.
func (st *State) RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error {
	offerUUID, err := applicationOfferUUID(st, offer.Name)
	if err != nil {
		return errors.Trace(err)
	}
	op := removePermissionOp(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)))

	err = st.db().RunTransaction([]txn.Op{op})
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("offer user %q does not exist", user.Id()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
