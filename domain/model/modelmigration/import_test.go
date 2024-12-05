// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/modelmigration"
	modelmigrationtesting "github.com/juju/juju/core/modelmigration/testing"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	modelService            *MockModelService
	readOnlyModelService    *MockReadOnlyModelService
	userService             *MockUserService
	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelService = NewMockModelService(ctrl)
	s.readOnlyModelService = NewMockReadOnlyModelService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

// TestModelMetadataInvalid tests that if we don't pass good values in model
// config for model name and uuid we get back an error that satisfies
// [errors.NotValid]
func (i *importSuite) TestModelMetadataInvalid(c *gc.C) {
	importOp := importOperation{}

	// model name not defined
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.UUIDKey: "test",
		},
	})
	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)

	// model name of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: 10,
			config.UUIDKey: "test",
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)

	// uuid not defined
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)

	// uuid of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: 11,
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestModelOwnerNoExist is asserting that if we try and import a model where
// the owner does not exist we get back a [usererrors.NotFound] error.
func (i *importSuite) TestModelOwnerNoExist(c *gc.C) {
	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(coreuser.User{}, usererrors.UserNotFound)

	importOp := importOperation{
		modelService: i.modelService,
		userService:  i.userService,
	}

	modelUUID := modeltesting.GenModelUUID(c)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: modelUUID.String(),
		},
		Owner: names.NewUserTag("tlm"),
	})
	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

func (i *importSuite) TestModelCreate(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)
	i.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil)

	args := model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        "AWS",
			CloudRegion:  "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "AWS",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID: modelUUID,
	}

	activated := false
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}

	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.readOnlyModelService.EXPECT().CreateModel(gomock.Any(), controllerUUID).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "AWS",
		CloudRegion: "region1",
		Owner:       names.NewUserTag("tlm"),
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("tlm"),
		Cloud: names.NewCloudTag("AWS"),
		Name:  "my-credential",
	})

	importOp := &importOperation{
		userService:              i.userService,
		modelService:             i.modelService,
		controllerConfigService:  i.controllerConfigService,
		readOnlyModelServiceFunc: func(_ coremodel.UUID) ReadOnlyModelService { return i.readOnlyModelService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activated, jc.IsTrue)
}

func (i *importSuite) TestModelCreateRollbacksOnFailure(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)
	i.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil)

	args := model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        "AWS",
			CloudRegion:  "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "AWS",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID: modelUUID,
	}

	var activated bool
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.readOnlyModelService.EXPECT().CreateModel(gomock.Any(), controllerUUID).Return(errors.New("boom"))
	i.modelService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coremodel.UUID, options ...model.DeleteModelOption) error {
		opts := model.DefaultDeleteModelOptions()
		for _, fn := range options {
			fn(opts)
		}
		c.Assert(opts.DeleteDB(), jc.IsTrue)
		return nil
	})
	i.readOnlyModelService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "AWS",
		CloudRegion: "region1",
		Owner:       names.NewUserTag("tlm"),
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("tlm"),
		Cloud: names.NewCloudTag("AWS"),
		Name:  "my-credential",
	})

	importOp := &importOperation{
		userService:              i.userService,
		modelService:             i.modelService,
		controllerConfigService:  i.controllerConfigService,
		readOnlyModelServiceFunc: func(_ coremodel.UUID) ReadOnlyModelService { return i.readOnlyModelService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the read-only model is
	// correctly saved.
	c.Check(activated, jc.IsTrue)
}

func (i *importSuite) TestModelCreateRollbacksOnFailureIgnoreNotFoundModel(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)
	i.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil)

	args := model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        "AWS",
			CloudRegion:  "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "AWS",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID: modelUUID,
	}

	activated := false
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.readOnlyModelService.EXPECT().CreateModel(gomock.Any(), controllerUUID).Return(errors.New("boom"))
	i.modelService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).Return(modelerrors.NotFound)
	i.readOnlyModelService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "AWS",
		CloudRegion: "region1",
		Owner:       names.NewUserTag("tlm"),
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("tlm"),
		Cloud: names.NewCloudTag("AWS"),
		Name:  "my-credential",
	})

	importOp := &importOperation{
		userService:              i.userService,
		modelService:             i.modelService,
		controllerConfigService:  i.controllerConfigService,
		readOnlyModelServiceFunc: func(_ coremodel.UUID) ReadOnlyModelService { return i.readOnlyModelService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the read-only model is
	// correctly saved.
	c.Check(activated, jc.IsTrue)
}

func (i *importSuite) TestModelCreateRollbacksOnFailureIgnoreNotFoundReadOnlyModel(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)
	i.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(testing.FakeControllerConfig(), nil)

	activated := false
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}

	args := model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        "AWS",
			CloudRegion:  "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "AWS",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID: modelUUID,
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.readOnlyModelService.EXPECT().CreateModel(gomock.Any(), controllerUUID).Return(errors.New("boom"))
	i.modelService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).Return(nil)
	i.readOnlyModelService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "AWS",
		CloudRegion: "region1",
		Owner:       names.NewUserTag("tlm"),
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("tlm"),
		Cloud: names.NewCloudTag("AWS"),
		Name:  "my-credential",
	})

	importOp := &importOperation{
		userService:              i.userService,
		modelService:             i.modelService,
		controllerConfigService:  i.controllerConfigService,
		readOnlyModelServiceFunc: func(_ coremodel.UUID) ReadOnlyModelService { return i.readOnlyModelService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the read-only model is
	// correctly saved.
	c.Check(activated, jc.IsTrue)
}
