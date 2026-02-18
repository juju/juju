// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/modelmigration"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/access/state"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	schematesting.ControllerSuite

	coordinator *coremodelmigration.Coordinator
	scope       coremodelmigration.Scope
	svc         *service.Service

	modelUUID              model.UUID
	adminUserUUID          user.UUID
	controllerPermissionID permission.ID
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	controllerUUID := s.SeedControllerUUID(c)

	s.controllerPermissionID, _ = permission.ParseTagForID(names.NewControllerTag(controllerUUID))
	s.adminUserUUID = tc.Must(c, user.NewUUID)
	s.modelUUID = tc.Must(c, model.NewUUID)

	s.coordinator = coremodelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))

	controllerFactory := func(context.Context) (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}

	s.scope = coremodelmigration.NewScope(controllerFactory, nil, nil, s.modelUUID)
	s.svc = service.NewService(
		state.NewState(controllerFactory, clock.WallClock, loggertesting.WrapCheckLog(c)), clock.WallClock,
	)

	adminUserName, _ := user.NewName("admin")
	_, _, _ = s.svc.AddUser(c.Context(), service.AddUserArg{
		UUID:        s.adminUserUUID,
		Name:        adminUserName,
		CreatorUUID: s.adminUserUUID,
		Permission: permission.AccessSpec{
			Target: s.controllerPermissionID,
			Access: permission.SuperuserAccess,
		},
	})

	c.Cleanup(func() {
		s.coordinator = nil
		s.svc = nil
		s.scope = coremodelmigration.Scope{}
		s.modelUUID = ""
		s.adminUserUUID = ""
		s.controllerPermissionID = permission.ID{}
	})
}

func (s *importSuite) TestOfferPermissionImport(c *tc.C) {
	// Arrange
	modelmigration.RegisterOfferAccessImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Arrange: add users on which offer permissions are set.
	joeUserUUID := s.addUserToController(c, "joe", permission.LoginAccess)
	simonUserUUID := s.addUserToController(c, "simon", permission.LoginAccess)

	// Arrange: set up the import data
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
		Config: map[string]interface{}{
			config.UUIDKey: s.modelUUID.String()},
	})
	appName := "foo"
	app := desc.AddApplication(description.ApplicationArgs{
		Name:     appName,
		CharmURL: "ch:foo-1",
	})
	offerOneUUID := tc.Must(c, uuid.NewUUID).String()
	offerOneName := "foo"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerOneUUID,
		OfferName:       offerOneName,
		Endpoints:       map[string]string{"db": "db"},
		ApplicationName: appName,
		ACL: map[string]string{
			"admin": "admin",
			"joe":   "consume",
			"simon": "read",
		},
	})
	offerTwoUUID := tc.Must(c, uuid.NewUUID).String()
	offerTwoName := "agent"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerTwoUUID,
		OfferName:       offerTwoName,
		Endpoints:       map[string]string{"cos-agent": "cos-agent"},
		ApplicationName: appName,
		ACL: map[string]string{
			"simon": "admin",
		},
	})

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	obtainedOfferPermissions := s.getOfferPermissions(c, "v_permission_offer")
	c.Check(obtainedOfferPermissions, tc.SameContents, []offerAccess{
		{GrantTo: s.adminUserUUID.String(), GrantOn: offerOneUUID, AccessType: "admin"},
		{GrantTo: joeUserUUID, GrantOn: offerOneUUID, AccessType: "consume"},
		{GrantTo: simonUserUUID, GrantOn: offerOneUUID, AccessType: "read"},
		{GrantTo: simonUserUUID, GrantOn: offerTwoUUID, AccessType: "admin"},
	})
}

func (s *importSuite) TestOfferPermissionRollback(c *tc.C) {
	// Arrange:
	modelmigration.RegisterOfferAccessImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	migrationtesting.RegisterFailingImport(s.coordinator)

	// Arrange: add users on which offer permissions are set.
	s.addUserToController(c, "joe", permission.LoginAccess)
	s.addUserToController(c, "simon", permission.LoginAccess)

	// Arrange: set up the import data
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
		Config: map[string]interface{}{
			config.UUIDKey: s.modelUUID.String()},
	})
	appName := "foo"
	app := desc.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	offerOneUUID := tc.Must(c, uuid.NewUUID).String()
	offerOneName := "foo"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerOneUUID,
		OfferName:       offerOneName,
		Endpoints:       map[string]string{"db": "db"},
		ApplicationName: appName,
		ACL: map[string]string{
			"admin": "admin",
			"joe":   "consume",
			"simon": "read",
		},
	})
	offerTwoUUID := tc.Must(c, uuid.NewUUID).String()
	offerTwoName := "agent"
	app.AddOffer(description.ApplicationOfferArgs{
		OfferUUID:       offerTwoUUID,
		OfferName:       offerTwoName,
		Endpoints:       map[string]string{"cos-agent": "cos-agent"},
		ApplicationName: appName,
		ACL: map[string]string{
			"simon": "admin",
		},
	})

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Check(err, tc.ErrorIs, migrationtesting.IntentionalImportFailure)
	s.checkRowCount(c, "v_permission_offer", 0)
}

func (s *importSuite) TestPermissionImport(c *tc.C) {
	// Arrange
	s.seedModel(c)
	modelmigration.RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))

	// Arrange: add users on which model permissions are set.
	joeUserUUID := s.addUserToController(c, "joe", permission.LoginAccess)
	simonUserUUID := s.addUserToController(c, "simon", permission.LoginAccess)

	// Arrange: set up the import data
	desc := description.NewModel(description.ModelArgs{
		Owner: "admin",
		Type:  string(model.IAAS),
		Config: map[string]interface{}{
			config.NameKey: "test-me",
			config.UUIDKey: s.modelUUID.String()},
	})
	desc.AddUser(description.UserArgs{
		Name:           "joe",
		CreatedBy:      "admin",
		DateCreated:    time.Now(),
		LastConnection: time.Now(),
		Access:         "write",
	})
	desc.AddUser(description.UserArgs{
		Name:           "simon",
		CreatedBy:      "admin",
		DateCreated:    time.Now(),
		LastConnection: time.Now(),
		Access:         "admin",
	})

	// Act
	err := s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	obtainedOfferPermissions := s.getOfferPermissions(c, "v_permission_model")
	c.Check(obtainedOfferPermissions, tc.SameContents, []offerAccess{
		{GrantTo: joeUserUUID, GrantOn: s.modelUUID.String(), AccessType: "write"},
		{GrantTo: simonUserUUID, GrantOn: s.modelUUID.String(), AccessType: "admin"},
	})
}

func (s *importSuite) seedModel(c *tc.C) {
	cloudUUID := tc.Must(c, uuid.NewUUID).String()
	credUUID := tc.Must(c, uuid.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
			VALUES (?, ?, 7, "test-endpoint", true)
		`, cloudUUID, "test-cloud")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud_auth_type (cloud_uuid, auth_type_id)
			VALUES (?, 0), (?, 2)
		`, cloudUUID, cloudUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud_credential (uuid, cloud_uuid, auth_type_id, owner_uuid, name, revoked, invalid)
			VALUES (?, ?, ?, ?, "foobar", false, false)
		`, credUUID, cloudUUID, 0, s.adminUserUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO model (uuid, name, qualifier, model_type_id, life_id, cloud_uuid)
			VALUES (?, "test", "prod", 0, 0, ?)
		`, s.modelUUID.String(), cloudUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) addUserToController(c *tc.C, name string, access permission.Access) string {
	userName, _ := user.NewName(name)
	userUUID, _, _ := s.svc.AddUser(c.Context(), service.AddUserArg{
		Name:        userName,
		CreatorUUID: s.adminUserUUID,
		Permission: permission.AccessSpec{
			Target: s.controllerPermissionID,
			Access: access,
		},
	})
	return userUUID.String()
}

type offerAccess struct {
	GrantOn    string `db:"grant_on"`
	GrantTo    string `db:"grant_to"`
	AccessType string `db:"access_type"`
}

// getOfferPermissions gets the permissions for the given table.
func (s *importSuite) getOfferPermissions(c *tc.C, table string) []offerAccess {
	stmt, err := sqlair.Prepare(fmt.Sprintf(`
SELECT * AS &offerAccess.*
FROM %s
`, table), offerAccess{})
	c.Assert(err, tc.ErrorIsNil)

	var access []offerAccess

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&access)
	})

	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) getting offer permissions: %s",
		errors.ErrorStack(err)))
	return access
}

// checkRowCount checks that the given table has the expected number of rows.
func (s *importSuite) checkRowCount(c *tc.C, table string, expected int) {
	obtained := -1
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	c.Check(obtained, tc.Equals, expected, tc.Commentf("count of %q rows", table))
}
