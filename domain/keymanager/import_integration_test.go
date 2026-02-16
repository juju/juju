// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessstate "github.com/juju/juju/domain/access/state"
	keymanagermodelmigration "github.com/juju/juju/domain/keymanager/modelmigration"
	"github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/keymanager/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ControllerSuite

	modelUUID     model.UUID
	adminUserUUID user.UUID
	otherUserUUID user.UUID
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.adminUserUUID = tc.Must(c, user.NewUUID)
	err := accessState.AddUser(
		c.Context(),
		s.adminUserUUID,
		user.AdminUserName,
		"admin",
		false,
		s.adminUserUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.otherUserUUID = tc.Must(c, user.NewUUID)
	err = accessState.AddUser(
		c.Context(),
		s.otherUserUUID,
		usertesting.GenNewName(c, "other-user"),
		"other-user",
		false,
		s.adminUserUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.modelUUID = modeltesting.CreateTestModelWithoutActivation(c, s.TxnRunnerFactory(), "foo")
}

func (s *importSuite) TestImportFromModelConfig(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
		Config: map[string]any{
			"uuid":            s.modelUUID.String(),
			"authorized-keys": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	keymanagermodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(s.TxnRunnerFactory(), nil, nil, s.modelUUID), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	keys, err := svc.ListPublicKeysForUser(c.Context(), s.adminUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.HasLen, 2)
	c.Assert(keys[0].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
	c.Assert(keys[1].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2")
}

func (s *importSuite) TestImportFromModelDescription(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
		Config: map[string]any{
			"uuid": s.modelUUID.String(),
		},
	})

	desc.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "admin",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})
	desc.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "other-user",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	keymanagermodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(s.TxnRunnerFactory(), nil, nil, s.modelUUID), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)

	adminKeys, err := svc.ListPublicKeysForUser(c.Context(), s.adminUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(adminKeys, tc.HasLen, 2)
	c.Assert(adminKeys[0].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
	c.Assert(adminKeys[1].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2")

	userKeys, err := svc.ListPublicKeysForUser(c.Context(), s.otherUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(userKeys, tc.HasLen, 2)
	c.Assert(userKeys[0].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
	c.Assert(userKeys[1].Key, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2")
}

func (s *importSuite) TestImportThatFailsRollsback(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
		Config: map[string]any{
			"uuid": s.modelUUID.String(),
		},
	})

	desc.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "admin",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})
	desc.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "other-user",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	keymanagermodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	migrationtesting.RegisterFailingImport(coordinator)
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(s.TxnRunnerFactory(), nil, nil, s.modelUUID), desc)
	c.Assert(err, tc.ErrorIs, migrationtesting.IntentionalImportFailure)

	svc := s.setupService(c)

	adminKeys, err := svc.ListPublicKeysForUser(c.Context(), s.adminUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(adminKeys, tc.HasLen, 0)

	userKeys, err := svc.ListPublicKeysForUser(c.Context(), s.otherUserUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(userKeys, tc.HasLen, 0)
}

func (s *importSuite) setupService(c *tc.C) *service.Service {
	return service.NewService(
		s.modelUUID,
		state.NewState(s.TxnRunnerFactory()),
	)
}
