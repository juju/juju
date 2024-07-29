// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"sort"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
	modelUUID      coremodel.UUID
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)

	s.modelUUID = modeltesting.CreateTestModelWithConfig(c, s.TxnRunnerFactory(), "test-model",
		modeltesting.TestModelConfig{Owner: "model-owner"})

	s.ensureUser(c, "42", "admin", "42", false) // model owner
	s.ensureUser(c, "123", "bob", "42", false)
}

func (s *stateSuite) TestGetModelUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: "admin",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				Key:        s.modelUUID.String(),
				ObjectType: corepermission.Model,
			},
			Access: corepermission.AdminAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: "bob",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				Key:        s.modelUUID.String(),
				ObjectType: corepermission.Model,
			},
			Access: corepermission.ReadAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := []access.ModelUserInfo{
		{
			Name:           "admin",
			DisplayName:    "admin",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           "bob",
			DisplayName:    "bob",
			Access:         corepermission.ReadAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           "model-owner",
			DisplayName:    "model-owner",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), "admin", s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name < modelUsers[j].Name
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersNonAdmin(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: "admin",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				Key:        s.modelUUID.String(),
				ObjectType: corepermission.Model,
			},
			Access: corepermission.AdminAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: "bob",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				Key:        s.modelUUID.String(),
				ObjectType: corepermission.Model,
			},
			Access: corepermission.ReadAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := []access.ModelUserInfo{
		{
			Name:           "bob",
			DisplayName:    "bob",
			Access:         corepermission.ReadAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), "bob", s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name < modelUsers[j].Name
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersExternalUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	s.ensureUser(c, "666", "everyone@external", "42", true)
	s.ensureUser(c, "777", "jim@external", "42", true)
	s.ensureUser(c, "888", "john@external", "42", true)

	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: "everyone@external",
		AccessSpec: corepermission.AccessSpec{
			Target: corepermission.ID{
				Key:        s.modelUUID.String(),
				ObjectType: corepermission.Model,
			},
			Access: corepermission.AdminAccess,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := []access.ModelUserInfo{
		{
			Name:           "jim@external",
			DisplayName:    "jim@external",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           "john@external",
			DisplayName:    "john@external",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           "model-owner",
			DisplayName:    "model-owner",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), "model-owner", s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name < modelUsers[j].Name
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersNoPermissions(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetModelUsers(context.Background(), "bob", s.modelUUID)
	c.Assert(err, jc.ErrorIs, accesserrors.PermissionNotValid)
}

func (s *stateSuite) TestGetModelUsersModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetModelUsers(context.Background(), "admin", "bad-uuid")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) ensureUser(c *gc.C, userUUID, name, createdByUUID string, external bool) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userUUID, name, name, external, false, createdByUUID, time.Now())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID, false)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
