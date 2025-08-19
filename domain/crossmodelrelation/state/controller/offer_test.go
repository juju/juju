// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"

	corepermission "github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/crossmodelrelation"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type controllerOfferSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
}

func TestControllerOfferSuite(t *testing.T) {
	tc.Run(t, &controllerOfferSuite{})
}

func (s *controllerOfferSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)
}

func (s *controllerOfferSuite) TestCreateOfferAccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	ownerUUID := uuid.MustNewUUID()
	everyoneUUID := "567"
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)
	s.ensureUser(c, everyoneUUID, corepermission.EveryoneUserName.String(), ownerUUID.String(), true, false, false)

	// Act
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)

	// Assert
	c.Assert(err, tc.IsNil)
	obtainedPermissions := s.readPermissions(c)
	c.Assert(obtainedPermissions, tc.HasLen, 2)
	expectedPermissions := []permission{
		{
			GrantTo:    ownerUUID.String(),
			GrantOn:    offerUUID.String(),
			AccessType: corepermission.AdminAccess.String(),
			ObjectType: corepermission.Offer.String(),
		}, {
			GrantTo:    everyoneUUID,
			GrantOn:    offerUUID.String(),
			AccessType: corepermission.ReadAccess.String(),
			ObjectType: corepermission.Offer.String(),
		},
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.Not(tc.HasLen), 0)
	c.Check(obtainedPermissions, tc.UnorderedMatch[[]permission](mc), expectedPermissions)
}

func (s *controllerOfferSuite) TestCreateOfferAccessEveryoneMissing(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	ownerUUID := uuid.MustNewUUID()
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)

	// Act
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*: user not found`)
}

func (s *controllerOfferSuite) TestCreateOfferAccessOwnerMissing(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	ownerUUID := uuid.MustNewUUID()

	// Act
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `.*: user not found`)
}

func (s *controllerOfferSuite) TestGetUserUUIDByName(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	userUUID := uuid.MustNewUUID()
	userName := usertesting.GenNewName(c, "fred")
	s.ensureUser(c, userUUID.String(), userName.Name(), userUUID.String(), false, false, false)

	// Act
	obtainedUserUUID, err := st.GetUserUUIDByName(c.Context(), userName)

	// Arrange
	c.Assert(err, tc.IsNil)
	c.Assert(obtainedUserUUID.String(), tc.Equals, userUUID.String())
}

func (s *controllerOfferSuite) TestGetUserUUIDByNameRemoved(c *tc.C) {
	s.testGetUserUUIDByNameNotFound(c, true, false, "user not found")
}

func (s *controllerOfferSuite) TestGetUserUUIDByNameDisabled(c *tc.C) {
	s.testGetUserUUIDByNameNotFound(c, false, true, "user authentication disabled")
}

func (s *controllerOfferSuite) testGetUserUUIDByNameNotFound(c *tc.C, removed, disabled bool, errMsg string) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	userUUID := uuid.MustNewUUID()
	userName := usertesting.GenNewName(c, "fred")
	s.ensureUser(c, userUUID.String(), userName.Name(), userUUID.String(), false, removed, disabled)

	// Act
	_, err := st.GetUserUUIDByName(c.Context(), userName)

	// Arrange
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`.*: %s`, errMsg))
}

func (s *controllerOfferSuite) TestGetOfferUUIDsForUsersWithConsume(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange:
	// Create two offers. Add consume permissions for a new user.

	// Create an offer permission with the owner
	ownerUUID := uuid.MustNewUUID()
	everyoneUUID := "567"
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)
	s.ensureUser(c, everyoneUUID, corepermission.EveryoneUserName.String(), ownerUUID.String(), true, false, false)
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Create second offer permission with the same owner
	ownerPermissionUUIDTwo := uuid.MustNewUUID()
	offerUUIDTwo := uuid.MustNewUUID()
	err = st.CreateOfferAccess(c.Context(), ownerPermissionUUIDTwo, offerUUIDTwo, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Add a new user with consume permission on the second offer
	userUUIDTwo := uuid.MustNewUUID().String()
	s.ensureUser(c, userUUIDTwo, "fred", ownerUUID.String(), false, false, false)
	s.addOfferPermission(c, userUUIDTwo, offerUUIDTwo.String(), 2)

	// Act
	obtained, err := st.GetOfferUUIDsForUsersWithConsume(c.Context(), []string{"fred"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.SameContents, []string{offerUUIDTwo.String()})
}

func (s *controllerOfferSuite) TestGetOfferUUIDsForUsersWithConsumeNoOffers(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange:
	// Create an offer permission with the owner
	ownerUUID := uuid.MustNewUUID()
	everyoneUUID := "567"
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)
	s.ensureUser(c, everyoneUUID, corepermission.EveryoneUserName.String(), ownerUUID.String(), true, false, false)
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Create second offer permission with the same owner
	ownerPermissionUUIDTwo := uuid.MustNewUUID()
	offerUUIDTwo := uuid.MustNewUUID()
	err = st.CreateOfferAccess(c.Context(), ownerPermissionUUIDTwo, offerUUIDTwo, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Act: try to find the offer for a user which does not exist.
	obtained, err := st.GetOfferUUIDsForUsersWithConsume(c.Context(), []string{"unknown"})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.SameContents, []string{})
}

func (s *controllerOfferSuite) TestGetUsersForOfferUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	// Create an offer permission with the owner
	ownerUUID := uuid.MustNewUUID()
	everyoneUUID := "567"
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)
	s.ensureUser(c, everyoneUUID, corepermission.EveryoneUserName.String(), ownerUUID.String(), true, false, false)
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Add two users with different permissions on the offer.
	// userOne has read permissions.
	// userTwo has consume permissions.
	userUUIDOne := uuid.MustNewUUID().String()
	s.ensureUser(c, userUUIDOne, "sam", ownerUUID.String(), false, false, false)
	s.addOfferPermission(c, userUUIDOne, offerUUID.String(), 0)
	userUUIDTwo := uuid.MustNewUUID().String()
	s.ensureUser(c, userUUIDTwo, "fred", ownerUUID.String(), false, false, false)
	s.addOfferPermission(c, userUUIDTwo, offerUUID.String(), 2)

	// Act
	result, err := st.GetUsersForOfferUUIDs(c.Context(), []string{offerUUID.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	resultNames, ok := result[offerUUID.String()]
	c.Check(ok, tc.Equals, true)
	c.Assert(resultNames, tc.SameContents, []crossmodelrelation.OfferUser{
		{Name: "sam", DisplayName: "sam", Access: "read"},
		{Name: "admin", DisplayName: "admin", Access: "admin"},
		{Name: "fred", DisplayName: "fred", Access: "consume"},
	})
}

func (s *controllerOfferSuite) TestGetUsersForOfferUUIDsNoUsers(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Arrange
	// Create an offer permission with the owner
	ownerUUID := uuid.MustNewUUID()
	everyoneUUID := "567"
	s.ensureUser(c, ownerUUID.String(), "admin", ownerUUID.String(), false, false, false)
	s.ensureUser(c, everyoneUUID, corepermission.EveryoneUserName.String(), ownerUUID.String(), true, false, false)
	ownerPermissionUUID := uuid.MustNewUUID()
	offerUUID := uuid.MustNewUUID()
	err := st.CreateOfferAccess(c.Context(), ownerPermissionUUID, offerUUID, ownerUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Remove permissions of the offer owner.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM permission WHERE object_type_id = 3`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act
	result, err := st.GetUsersForOfferUUIDs(c.Context(), []string{offerUUID.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	resultOfferNames, ok := result[offerUUID.String()]
	c.Check(ok, tc.Equals, true, tc.Commentf("%+v", resultOfferNames))
	c.Assert(resultOfferNames, tc.SameContents, []crossmodelrelation.OfferUser{})
}

func (s *controllerOfferSuite) ensureUser(c *tc.C, userUUID, name, createdByUUID string, external, removed, disabled bool) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userUUID, name, name, external, removed, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID, disabled)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerOfferSuite) addOfferPermission(c *tc.C, userUUID, offerUUID string, accessType int) string {
	permissionUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
			VALUES (?, ?, ?, ?, ?)
		`, permissionUUID, accessType, 3, userUUID, offerUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return permissionUUID
}

func (s *controllerOfferSuite) readPermissions(c *tc.C) []permission {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM v_permission`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundPermissions := []permission{}
	for rows.Next() {
		var p permission
		err = rows.Scan(&p.UUID, &p.GrantOn, &p.GrantTo, &p.AccessType, &p.ObjectType)
		c.Assert(err, tc.IsNil)
		foundPermissions = append(foundPermissions, p)
	}
	return foundPermissions
}
