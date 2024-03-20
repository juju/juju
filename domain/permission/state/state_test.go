// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/permission"
	permissionerrors "github.com/juju/juju/domain/permission/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestCreatePermissionModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureModel(c, "model-uuid")
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")
	s.ensureCloud(c, "test-cloud")

	userAccess, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "model-uuid",
			ObjectType: corepermission.Model,
		},
		Access: corepermission.WriteAccess,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(userAccess.UserID, gc.Equals, "123")
	c.Check(userAccess.UserTag, gc.Equals, names.NewUserTag("bob"))
	c.Check(userAccess.Object.Id(), gc.Equals, "model-uuid")
	c.Check(userAccess.Access, gc.Equals, corepermission.WriteAccess)
	c.Check(userAccess.DisplayName, gc.Equals, "Bob")
	c.Check(userAccess.UserName, gc.Equals, "bob")
	c.Check(userAccess.CreatedBy, gc.Equals, names.NewUserTag("admin"))

	s.checkPermissionRow(c, corepermission.WriteAccess, "123", "model-uuid")
}

func (s *stateSuite) TestCreatePermissionCloud(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureModel(c, "model-uuid")
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")
	s.ensureCloud(c, "test-cloud")

	userAccess, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "test-cloud",
			ObjectType: corepermission.Cloud,
		},
		Access: corepermission.AddModelAccess,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(userAccess.UserID, gc.Equals, "123")
	c.Check(userAccess.UserTag, gc.Equals, names.NewUserTag("bob"))
	c.Check(userAccess.Object.Id(), gc.Equals, "test-cloud")
	c.Check(userAccess.Access, gc.Equals, corepermission.AddModelAccess)
	c.Check(userAccess.DisplayName, gc.Equals, "Bob")
	c.Check(userAccess.UserName, gc.Equals, "bob")
	c.Check(userAccess.CreatedBy, gc.Equals, names.NewUserTag("admin"))

	s.checkPermissionRow(c, corepermission.AddModelAccess, "123", "test-cloud")
}

func (s *stateSuite) TestCreatePermissionController(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	userAccess, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "controller",
			ObjectType: corepermission.Controller,
		},
		Access: corepermission.SuperuserAccess,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(userAccess.UserID, gc.Equals, "123")
	c.Check(userAccess.UserTag, gc.Equals, names.NewUserTag("bob"))
	c.Check(userAccess.Object.Id(), gc.Equals, "controller")
	c.Check(userAccess.Access, gc.Equals, corepermission.SuperuserAccess)
	c.Check(userAccess.DisplayName, gc.Equals, "Bob")
	c.Check(userAccess.UserName, gc.Equals, "bob")
	c.Check(userAccess.CreatedBy, gc.Equals, names.NewUserTag("admin"))

	s.checkPermissionRow(c, corepermission.SuperuserAccess, "123", "controller")
}

func (s *stateSuite) TestCreatePermissionForModelWithBadInfo(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "foo-bar",
			ObjectType: corepermission.Model,
		},
		Access: corepermission.ReadAccess,
	})
	c.Assert(err, jc.ErrorIs, permissionerrors.TargetInvalid)
}

func (s *stateSuite) TestCreatePermissionForControllerWithBadInfo(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "foo-bar",
			ObjectType: corepermission.Controller,
		},
		Access: corepermission.SuperuserAccess,
	})
	c.Assert(err, jc.ErrorIs, permissionerrors.TargetInvalid)
}

func (s *stateSuite) checkPermissionRow(c *gc.C, access corepermission.Access, expectedGrantTo, expectedGrantON string) {
	db := s.DB()
	// Find the id for access
	accessRow := db.QueryRow(`
SELECT id
FROM permission_access_type
WHERE type = ?
`, access)
	c.Assert(accessRow.Err(), jc.ErrorIsNil)
	var accessTypeID int
	err := accessRow.Scan(&accessTypeID)
	c.Assert(err, jc.ErrorIsNil)

	// Find the permission
	row := db.QueryRow(`
SELECT uuid, permission_type_id, grant_to, grant_on 
FROM permission
`)
	c.Assert(row.Err(), jc.ErrorIsNil)
	var (
		userUuid, grantTo, grantOn string
		permissionTypeId           int
	)
	err = row.Scan(&userUuid, &permissionTypeId, &grantTo, &grantOn)
	c.Assert(err, jc.ErrorIsNil)

	// Verify the permission as expected.
	c.Check(userUuid, gc.Not(gc.Equals), "")
	c.Check(permissionTypeId, gc.Equals, accessTypeID)
	c.Check(grantTo, gc.Equals, expectedGrantTo)
	c.Check(grantOn, gc.Equals, expectedGrantON)
}

func (s *stateSuite) TestCreatePermissionErrorNoUser(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "model-uuid",
			ObjectType: corepermission.Model,
		},
		Access: corepermission.WriteAccess,
	})
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (s *stateSuite) TestCreatePermissionErrorDuplicate(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureModel(c, "model-uuid")
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	spec := permission.UserAccessSpec{
		User: "bob",
		Target: corepermission.ID{
			Key:        "model-uuid",
			ObjectType: corepermission.Model,
		},
		Access: corepermission.ReadAccess,
	}
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), spec)
	c.Assert(err, jc.ErrorIsNil)

	// Find the permission
	row := s.DB().QueryRow(`
SELECT uuid, permission_type_id, grant_to, grant_on 
FROM permission
WHERE permission_type_id = 0
`)
	c.Assert(row.Err(), jc.ErrorIsNil)

	var (
		userUuid, grantTo, grantOn string
		permissionTypeId           int
	)
	err = row.Scan(&userUuid, &permissionTypeId, &grantTo, &grantOn)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure each combination of grant_on and grant_two
	// is unique
	spec.Access = corepermission.WriteAccess
	ua2, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), spec)
	c.Assert(err, jc.ErrorIs, permissionerrors.AlreadyExists)
	c.Logf("B +%v", ua2)
	row2 := s.DB().QueryRow(`
SELECT uuid, permission_type_id, grant_to, grant_on 
FROM permission
WHERE permission_type_id = 1
`)
	c.Assert(row2.Err(), jc.ErrorIsNil)
	err = row2.Scan(&userUuid, &permissionTypeId, &grantTo, &grantOn)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestDeletePermission(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureModel(c, "model-uuid")
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	target := corepermission.ID{
		Key:        "model-uuid",
		ObjectType: corepermission.Model,
	}
	spec := permission.UserAccessSpec{
		User:   "bob",
		Target: target,
		Access: corepermission.ReadAccess,
	}
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), spec)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeletePermission(context.Background(), "bob", target)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()

	var num int
	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM permission").Scan(&num)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(num, gc.Equals, 0)
}

func (s *stateSuite) TestDeletePermissionFailUserNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	target := corepermission.ID{
		Key:        "model-uuid",
		ObjectType: corepermission.Model,
	}
	err := st.DeletePermission(context.Background(), "bob", target)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (s *stateSuite) TestDeletePermissionDoesNotExist(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))

	// Setup to add permissions for user Bob on the model
	s.ensureModel(c, "model-uuid")
	s.ensureUser(c, "42", "admin", "42") // model owner
	s.ensureUser(c, "123", "bob", "42")

	target := corepermission.ID{
		Key:        "model-uuid",
		ObjectType: corepermission.Model,
	}

	// Don't fail if the permission does not exist.
	err := st.DeletePermission(context.Background(), "bob", target)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) ensureUser(c *gc.C, uuid, name, createdByUUID string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, uuid, name, "Bob", false, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) ensureModel(c *gc.C, uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model_list (uuid)
			VALUES (?)
		`, uuid)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) ensureCloud(c *gc.C, name string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
			VALUES (?, ?, 1, "test-endpoint", true)
		`, "cloud-uuid", name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
