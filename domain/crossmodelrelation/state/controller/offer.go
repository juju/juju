// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
// ReadAccess for the provided offer.
func (st *State) CreateOfferAccess(ctx context.Context, permissionUUID, offerUUID, ownerUUID internaluuid.UUID) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	everyonePermissionUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	ownerPermission := permission{
		UUID:       permissionUUID.String(),
		GrantOn:    offerUUID.String(),
		GrantTo:    ownerUUID.String(),
		AccessType: corepermission.AdminAccess.String(),
		ObjectType: corepermission.Offer.String(),
	}
	everyonePermission := permission{
		UUID:       everyonePermissionUUID.String(),
		GrantOn:    offerUUID.String(),
		AccessType: corepermission.ReadAccess.String(),
		ObjectType: corepermission.Offer.String(),
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		everyoneExternalUUID, err := st.getUserUUIDByName(ctx, tx, corepermission.EveryoneUserName)
		if errors.Is(err, accesserrors.UserNotFound) || errors.Is(err, accesserrors.UserAuthenticationDisabled) {
			return errors.Errorf("%q (should be added on bootstrap): %w", corepermission.EveryoneUserName, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		if err := insertPermission(ctx, tx, ownerPermission); err != nil {
			return errors.Capture(err)
		}

		everyonePermission.GrantTo = everyoneExternalUUID
		err = insertPermission(ctx, tx, everyonePermission)
		return errors.Capture(err)
	})

	return errors.Capture(err)
}

// Note: insertPermission is borrowed from the access domain.
func insertPermission(ctx context.Context, tx *sqlair.TX, perm permission) error {
	// Insert a permission doc with
	// * id of access type as access_type_id
	// * id of object type as object_type_id
	// * uuid of the user (spec.User) as grant_to
	// * spec.Target.Key as grant_on
	newPermission := `
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
SELECT $permission.uuid,
       at.id,
       ot.id,
       u.uuid,
       $permission.grant_on
FROM   v_user_auth u,
       permission_access_type at,
       permission_object_type ot
WHERE  u.uuid = $permission.grant_to
AND    u.disabled = false
AND    u.removed = false
AND    at.type = $permission.access_type
AND    ot.type = $permission.object_type
`
	insertPermissionStmt, err := sqlair.Prepare(newPermission, permission{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if err != nil {
		return errors.Errorf("adding permission %q for %q on %q: %w", perm.AccessType, perm.GrantTo, perm.GrantOn, err)
	}
	return nil
}

// GetUserUUIDByName returns the UUID of the user provided exists, has not
// been removed and is not disabled.
func (st *State) GetUserUUIDByName(ctx context.Context, userName coreuser.Name) (internaluuid.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internaluuid.UUID{}, errors.Capture(err)
	}
	var userUUID string

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		userUUID, err = st.getUserUUIDByName(ctx, tx, userName)
		return err
	})

	if err != nil {
		return internaluuid.UUID{}, errors.Capture(err)
	}

	result, err := internaluuid.UUIDFromString(userUUID)
	if err != nil {
		return internaluuid.UUID{}, errors.Capture(err)
	}
	return result, nil
}

// getUserUUIDByName finds the user UUID provided exists, hasn't been removed
// and is not disabled.
func (st *State) getUserUUIDByName(ctx context.Context, tx *sqlair.TX, userName coreuser.Name) (string, error) {
	var result user

	uName := name{Name: userName.Name()}

	getUserQuery := `
SELECT (u.uuid, u.disabled) AS (&user.*)
FROM   v_user_auth AS u
WHERE  u.removed = false
       AND u.name = $name.name`

	selectUserStmt, err := st.Prepare(getUserQuery, user{}, uName)
	if err != nil {
		return "", errors.Errorf("preparing select getUser query: %w", err)
	}
	err = tx.Query(ctx, selectUserStmt, uName).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%q: %w", userName, accesserrors.UserNotFound)
	} else if err != nil {
		return "", errors.Errorf("getting user with name %q: %w", userName, err)
	}
	if result.Disabled {
		return "", errors.Errorf("%q: %w", userName, accesserrors.UserAuthenticationDisabled)
	}
	return result.UUID, nil
}

// GetOfferUUIDsForUsersWithConsume returns offer uuids for any of the
// given users whom has consumer access or greater. It returns found
// Offers, with guarantee that offers for all users have been found.
func (st *State) GetOfferUUIDsForUsersWithConsume(
	ctx context.Context,
	userNames []string,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type names []string

	stmt, err := st.Prepare(`
SELECT p.grant_on AS &entityUUID.uuid
FROM   v_permission_offer AS p
JOIN   v_user_auth AS u ON p.grant_to = u.uuid
WHERE  u.name IN ($names[:])
AND    u.disabled = false
AND    u.removed = false
AND    (p.access_type = 'consume' OR p.access_type = 'admin')
`, names{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing get user with permission query: %w", err)
	}

	var results []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, names(userNames)).GetAll(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting offers for users %q: %w", strings.Join(userNames, ", "), err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	offerUUIDs := transform.Slice(
		results,
		func(in entityUUID) string { return in.UUID },
	)

	// Use set.Strings to ensure there are no duplicate offer UUIDs.
	return set.NewStrings(offerUUIDs...).Values(), nil
}

// GetUsersForOfferUUIDs returns a map of offerUUIDs with a slice of users
// whom have permissions on the offer. A map of offerUUIDs to a slice of
// OfferUsers is returned.
func (st *State) GetUsersForOfferUUIDs(ctx context.Context, offerUUIDs []string) (map[string][]crossmodelrelation.OfferUser, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type uuids []string

	stmt, err := st.Prepare(`
SELECT * AS &offerUser.*
FROM   v_permission_offer AS p
JOIN   v_user_auth AS u ON p.grant_to = u.uuid
WHERE  p.grant_on IN ($uuids[:])
AND    u.disabled = false
AND    u.removed = false
`, uuids{}, offerUser{})
	if err != nil {
		return nil, errors.Errorf("preparing get user with permission query: %w", err)
	}

	var offerUsers []offerUser
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, uuids(offerUUIDs)).GetAll(&offerUsers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting users for offers %q: %w", strings.Join(offerUUIDs, ", "), err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// There are scenarios where an offer can have no users with direct
	// permissions. Though a model admin always has implicit admin
	// permissions on any offers in the model per the facade. Ensure
	// that all requested offers are returned, though users are not
	// guaranteed.
	results := transform.SliceToMap(offerUUIDs, func(in string) (string, []crossmodelrelation.OfferUser) {
		return in, make([]crossmodelrelation.OfferUser, 0)
	})

	for _, one := range offerUsers {
		// Everyone is added by default for users not defined in the
		// database. Do not return it.
		if one.Name == corepermission.EveryoneUserName.String() {
			continue
		}
		offerUser := crossmodelrelation.OfferUser{
			Name:        one.Name,
			DisplayName: one.DisplayName,
			Access:      corepermission.Access(one.Access),
		}
		results[one.OfferUUID] = append(results[one.OfferUUID], offerUser)
	}
	return results, nil
}
