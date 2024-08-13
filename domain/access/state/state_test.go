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
	coreusertesting "github.com/juju/juju/core/user/testing"
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

	s.modelUUID = modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")

	s.ensureUser(c, "42", "admin", "42", false) // model owner
	s.ensureUser(c, "123", "bob", "42", false)
}

func (s *stateSuite) TestGetModelUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	adminName := coreusertesting.GenNewName(c, "admin")
	bobName := coreusertesting.GenNewName(c, "bob")
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: adminName,
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
		User: bobName,
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
			Name:           adminName,
			DisplayName:    adminName.Name(),
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           bobName,
			DisplayName:    bobName.Name(),
			Access:         corepermission.ReadAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           coreusertesting.GenNewName(c, "test-usertest-model"),
			DisplayName:    "test-usertest-model",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), adminName, s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name.Name() < modelUsers[j].Name.Name()
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersNonAdmin(c *gc.C) {
	adminName := coreusertesting.GenNewName(c, "admin")
	bobName := coreusertesting.GenNewName(c, "bob")
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: adminName,
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
		User: bobName,
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
			Name:           bobName,
			DisplayName:    "bob",
			Access:         corepermission.ReadAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), bobName, s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name.Name() < modelUsers[j].Name.Name()
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersExternalUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	everyoneName := corepermission.EveryoneUserName
	jimName := coreusertesting.GenNewName(c, "jim@external")
	johnName := coreusertesting.GenNewName(c, "john@external")
	s.ensureUser(c, "666", everyoneName.Name(), "42", true)
	s.ensureUser(c, "777", jimName.Name(), "42", true)
	s.ensureUser(c, "888", johnName.Name(), "42", true)

	_, err := st.CreatePermission(context.Background(), uuid.MustNewUUID(), corepermission.UserAccessSpec{
		User: everyoneName,
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
			Name:           jimName,
			DisplayName:    jimName.Name(),
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           johnName,
			DisplayName:    johnName.Name(),
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           coreusertesting.GenNewName(c, "test-usertest-model"),
			DisplayName:    "test-usertest-model",
			Access:         corepermission.AdminAccess,
			LastModelLogin: time.Time{},
		},
	}

	modelUsers, err := st.GetModelUsers(context.Background(), coreusertesting.GenNewName(c, "test-usertest-model"), s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(modelUsers, func(i, j int) bool {
		return modelUsers[i].Name.Name() < modelUsers[j].Name.Name()
	})
	c.Assert(modelUsers, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsersNoPermissions(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetModelUsers(context.Background(), coreusertesting.GenNewName(c, "bob"), s.modelUUID)
	c.Assert(err, jc.ErrorIs, accesserrors.PermissionNotValid)
}

func (s *stateSuite) TestGetModelUsersModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetModelUsers(context.Background(), coreusertesting.GenNewName(c, "admin"), "bad-uuid")
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
