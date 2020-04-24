// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/permission"
)

// GetOfferAccess gets the access permission for the specified user on an offer.
func (st *State) GetOfferAccess(offerUUID string, user names.UserTag) (permission.Access, error) {
	perm, err := st.userPermission(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)))
	if err != nil {
		return "", errors.Trace(err)
	}
	return perm.access(), nil
}

// GetOfferUsers gets the access permissions on an offer.
func (st *State) GetOfferUsers(offerUUID string) (map[string]permission.Access, error) {
	perms, err := st.usersPermissions(applicationOfferKey(offerUUID))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]permission.Access)
	for _, p := range perms {
		result[userIDFromGlobalKey(p.doc.SubjectGlobalKey)] = p.access()
	}
	return result, nil
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

	buildTxn := func(int) ([]txn.Op, error) {
		_, err := st.GetOfferAccess(offerUUID, user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		isAdmin, err := st.isControllerOrModelAdmin(user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := []txn.Op{updatePermissionOp(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)), access)}
		if !isAdmin && access != permission.ConsumeAccess && access != permission.AdminAccess {
			suspendOps, err := st.suspendRevokedRelationsOps(offerUUID, user.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, suspendOps...)
		}
		return ops, nil
	}

	err = st.db().Run(buildTxn)
	return errors.Trace(err)
}

// suspendRevokedRelationsOps suspends any relations the given user has against
// the specified offer.
func (st *State) suspendRevokedRelationsOps(offerUUID, userId string) ([]txn.Op, error) {
	conns, err := st.OfferConnections(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var ops []txn.Op
	relIdsToSuspend := make(set.Ints)
	for _, oc := range conns {
		if oc.UserName() == userId {
			rel, err := st.Relation(oc.RelationId())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if rel.Suspended() {
				continue
			}
			relIdsToSuspend.Add(rel.Id())
			suspendOp := txn.Op{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"suspended", true}}}},
			}
			ops = append(ops, suspendOp)
		}
	}

	// Add asserts that the relations against the offered application don't change.
	// This is broader than what we need but it's all that's possible.
	ao := NewApplicationOffers(st)
	offer, err := ao.ApplicationOfferForUUID(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	app, err := st.Application(offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	relations, err := app.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sameRelCount := bson.D{{"relationcount", len(relations)}}
	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     app.doc.DocID,
		Assert: sameRelCount,
	})
	// Ensure any relations not being updated still exist.
	for _, r := range relations {
		if relIdsToSuspend.Contains(r.Id()) {
			continue
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     r.doc.DocID,
			Assert: txn.DocExists,
		})
	}

	return ops, nil
}

// RemoveOfferAccess removes the access permission for a user on an offer.
func (st *State) RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error {
	offerUUID, err := applicationOfferUUID(st, offer.Name)
	if err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(int) ([]txn.Op, error) {
		_, err := st.GetOfferAccess(offerUUID, user)
		if err != nil {
			return nil, err
		}
		isAdmin, err := st.isControllerOrModelAdmin(user)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := []txn.Op{removePermissionOp(applicationOfferKey(offerUUID), userGlobalKey(userAccessID(user)))}
		if !isAdmin {
			suspendOps, err := st.suspendRevokedRelationsOps(offerUUID, user.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, suspendOps...)
		}
		return ops, nil
	}

	err = st.db().Run(buildTxn)
	return errors.Trace(err)
}
