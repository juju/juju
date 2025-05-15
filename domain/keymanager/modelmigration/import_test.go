// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
	userService *MockUserService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.userService = NewMockUserService(ctrl)
	return ctrl
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		serviceGetter: func(_ coremodel.UUID) ImportService {
			return s.service
		},
		userService: s.userService,
	}
}

// TestImportFromModelConfig is asserting that if model config contains
// authorized-keys that the import code correctly finds and handles them.
func (s *importSuite) TestImportFromModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{

		Config: map[string]any{
			"uuid":            modeltesting.GenModelUUID(c).String(),
			"authorized-keys": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})

	userId := usertesting.GenUserUUID(c)
	s.userService.EXPECT().GetUserByName(gomock.Any(), user.AdminUserName).Return(user.User{
		UUID: userId,
	}, nil)

	s.service.EXPECT().AddPublicKeysForUser(gomock.Any(), userId, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Check(err, tc.ErrorIsNil)
}

// TestImportFromModelDescription is responsible for asserting that we can
// import authorized keys for a model directly from the description and not
// model config.
func (s *importSuite) TestImportFromModelDescription(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": modeltesting.GenModelUUID(c).String(),
		},
	})
	model.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "tlm",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})
	model.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
		Username: "wallyworld",
		AuthorizedKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
		},
	})

	userIdTLM := usertesting.GenUserUUID(c)
	userIdWallyworld := usertesting.GenUserUUID(c)
	usernameTLM := usertesting.GenNewName(c, "tlm")
	usernameWallyworld := usertesting.GenNewName(c, "wallyworld")
	s.userService.EXPECT().GetUserByName(gomock.Any(), usernameTLM).Return(user.User{
		UUID: userIdTLM,
	}, nil)
	s.userService.EXPECT().GetUserByName(gomock.Any(), usernameWallyworld).Return(user.User{
		UUID: userIdWallyworld,
	}, nil)

	s.service.EXPECT().AddPublicKeysForUser(gomock.Any(), userIdTLM, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	})
	s.service.EXPECT().AddPublicKeysForUser(gomock.Any(), userIdWallyworld, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Check(err, tc.ErrorIsNil)
}
