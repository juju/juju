// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/offer"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
// ReadAccess for the provided offer.
func (st *State) CreateOfferAccess(
	ctx context.Context,
	permissionUUID internaluuid.UUID,
	offerUUID offer.UUID,
	ownerUUID internaluuid.UUID,
) error {
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		everyoneExternalUUID, err := st.getUserUUIDByName(ctx, tx, corepermission.EveryoneUserName)
		if errors.Is(err, accesserrors.UserNotFound) ||
			errors.Is(err, accesserrors.UserAuthenticationDisabled) {
			return errors.Errorf("%q (should be added on bootstrap): %w", corepermission.EveryoneUserName, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		// Insert the owner permission.
		if err := insertPermission(ctx, tx, ownerPermission); err != nil {
			return errors.Errorf("inserting owner permission: %w", err)
		}

		// Insert the everyone permission.
		everyonePermission := permission{
			UUID:       everyonePermissionUUID.String(),
			GrantOn:    offerUUID.String(),
			AccessType: corepermission.ReadAccess.String(),
			ObjectType: corepermission.Offer.String(),
			GrantTo:    everyoneExternalUUID,
		}
		if err := insertPermission(ctx, tx, everyonePermission); err != nil {
			return errors.Errorf("inserting everyone permission: %w", err)
		}
		return nil
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

// UpdateOfferPermission updates the access permission for the specified
// user on the given offer. It handles both granting and revoking access,
// following the same semantics as the access domain's UpdatePermission
// for offer targets. The permissionUUID is persisted when the grant
// creates a new permission for a user without one on the offer.
func (st *State) UpdateOfferPermission(
	ctx context.Context,
	permissionUUID string,
	args crossmodelrelation.UpdateOfferPermissionArgs,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		subjectUUID, err := st.getUserUUIDByName(ctx, tx, args.Username)
		if err != nil {
			return errors.Errorf("looking up user %q: %w", args.Username, err)
		}

		switch args.Change {
		case corepermission.Grant:
			return st.grantOfferPermission(ctx, tx, permissionUUID, subjectUUID, args)
		case corepermission.Revoke:
			return st.revokeOfferPermission(ctx, tx, args)
		default:
			return errors.Errorf("unsupported change type %q", args.Change)
		}
	})
	return errors.Capture(err)
}

func (st *State) grantOfferPermission(
	ctx context.Context,
	tx *sqlair.TX,
	permissionUUID string,
	subjectUUID string,
	args crossmodelrelation.UpdateOfferPermissionArgs,
) error {
	inOut := permInOut{
		Name:    args.Username.Name(),
		GrantOn: args.OfferUUID,
	}

	readStmt, err := st.Prepare(`
SELECT p.access_type AS &permInOut.access_type
FROM   v_permission_offer AS p
JOIN   v_user_auth AS u ON p.grant_to = u.uuid
WHERE  u.name = $permInOut.name
AND    u.disabled = false
AND    u.removed = false
AND    p.grant_on = $permInOut.grant_on
`, inOut)
	if err != nil {
		return errors.Errorf("preparing read current offer permission: %w", err)
	}

	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if errors.Is(err, sqlair.ErrNoRows) {
		perm := permission{
			UUID:       permissionUUID,
			GrantOn:    args.OfferUUID,
			GrantTo:    subjectUUID,
			AccessType: args.Access.String(),
			ObjectType: corepermission.Offer.String(),
		}
		return insertPermission(ctx, tx, perm)
	} else if err != nil {
		return errors.Errorf("reading current offer permission: %w", err)
	}

	spec := corepermission.AccessSpec{
		Target: corepermission.ID{
			ObjectType: corepermission.Offer,
			Key:        args.OfferUUID,
		},
		Access: corepermission.Access(inOut.Access),
	}

	if spec.EqualOrGreaterThan(args.Access) {
		return errors.Errorf("user %q already has %q %w", args.Username, args.Access, accesserrors.PermissionAccessGreater)
	}

	return st.updateOfferPermission(ctx, tx, args.Username.Name(), args.OfferUUID, args.Access.String())
}

func (st *State) revokeOfferPermission(
	ctx context.Context,
	tx *sqlair.TX,
	args crossmodelrelation.UpdateOfferPermissionArgs,
) error {
	inOut := permInOut{
		Name:    args.Username.Name(),
		GrantOn: args.OfferUUID,
	}
	readStmt, err := st.Prepare(`
SELECT p.access_type AS &permInOut.access_type
FROM   v_permission_offer AS p
JOIN   v_user_auth AS u ON p.grant_to = u.uuid
WHERE  u.name = $permInOut.name
AND    u.disabled = false
AND    u.removed = false
AND    p.grant_on = $permInOut.grant_on
`, inOut)
	if err != nil {
		return errors.Errorf("preparing read current offer permission: %w", err)
	}
	err = tx.Query(ctx, readStmt, inOut).Get(&inOut)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("offer permission for %q on %q %w", args.Username, args.OfferUUID, accesserrors.PermissionNotFound)
	} else if err != nil {
		return errors.Errorf("reading current offer permission: %w", err)
	}

	spec := corepermission.AccessSpec{
		Target: corepermission.ID{
			ObjectType: corepermission.Offer,
			Key:        args.OfferUUID,
		},
		Access: args.Access,
	}
	newAccess := spec.RevokeAccess()
	if newAccess == corepermission.NoAccess {
		return st.deleteOfferPermission(ctx, tx, args.Username.Name(), args.OfferUUID)
	}
	return st.updateOfferPermission(ctx, tx, args.Username.Name(), args.OfferUUID, newAccess.String())
}

func (st *State) updateOfferPermission(
	ctx context.Context,
	tx *sqlair.TX,
	subjectName, grantOn, access string,
) error {
	inOut := permInOut{
		Name:    subjectName,
		GrantOn: grantOn,
		Access:  access,
	}

	updateStmt, err := st.Prepare(`
UPDATE permission
SET    access_type_id = (
           SELECT id
           FROM   permission_access_type
           WHERE  type = $permInOut.access_type
       )
WHERE  grant_on = $permInOut.grant_on
AND    grant_to IN (
           SELECT uuid
           FROM   v_user_auth
           WHERE  name = $permInOut.name
           AND    removed = false
           AND    disabled = false
       )
`, inOut)
	if err != nil {
		return errors.Errorf("preparing update offer permission: %w", err)
	}

	if err := tx.Query(ctx, updateStmt, inOut).Run(); err != nil {
		return errors.Errorf("updating offer permission for %q on %q to %q: %w", subjectName, grantOn, access, err)
	}
	return nil
}

func (st *State) deleteOfferPermission(
	ctx context.Context,
	tx *sqlair.TX,
	subjectName, grantOn string,
) error {
	inOut := permInOut{
		Name:    subjectName,
		GrantOn: grantOn,
	}

	deleteStmt, err := st.Prepare(`
DELETE FROM permission
WHERE  grant_on = $permInOut.grant_on
AND    grant_to IN (
           SELECT uuid
           FROM   v_user_auth
           WHERE  name = $permInOut.name
           AND    removed = false
           AND    disabled = false
       )
`, inOut)
	if err != nil {
		return errors.Errorf("preparing delete offer permission: %w", err)
	}

	err = tx.Query(ctx, deleteStmt, inOut).Run()
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("deleting offer permission for %q on %q: %w", subjectName, grantOn, err)
	}
	return nil
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

// IsUserControllerOrModelAdmin returns true if the user has superuser
// access on the controller or admin access on the given model.
func (st *State) IsUserControllerOrModelAdmin(
	ctx context.Context,
	userName coreuser.Name,
	modelUUIDStr coremodel.UUID,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	type adminCheck struct {
		Check string `db:"check"`
	}

	user := name{Name: userName.Name()}
	model := modelUUID{ModelUUID: modelUUIDStr.String()}

	adminStmt, err := st.Prepare(`
SELECT 'x' AS &adminCheck.check
FROM   v_user_auth u
JOIN   v_permission p ON u.uuid = p.grant_to
WHERE  u.name = $name.name
AND    u.disabled = false
AND    u.removed = false
AND    (
           (p.object_type = 'controller' AND p.access_type = 'superuser')
           OR
           (p.object_type = 'model' AND p.grant_on = $modelUUID.model_uuid AND p.access_type IN ('admin', 'superuser'))
       )
LIMIT 1
`, adminCheck{}, name{}, modelUUID{})
	if err != nil {
		return false, errors.Errorf("preparing admin check: %w", err)
	}

	var check adminCheck
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, adminStmt, user, model).Get(&check)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return false, errors.Capture(err)
	}
	return check.Check == "x", nil
}
