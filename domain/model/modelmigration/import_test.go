// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
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
	modelImportService      *MockModelImportService
	modelDetailService      *MockModelDetailService
	userService             *MockUserService
	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelImportService = NewMockModelImportService(ctrl)
	s.modelDetailService = NewMockModelDetailService(ctrl)
	s.userService = NewMockUserService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	return ctrl
}

// TestModelMetadataInvalid tests that if we don't pass good values in model
// config for model name and uuid we get back an error that satisfies
// [errors.NotValid]
func (i *importSuite) TestModelMetadataInvalid(c *gc.C) {
	importOp := importModelOperation{}

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

	importOp := importModelOperation{
		modelImportService: i.modelImportService,
		userService:        i.userService,
	}

	modelUUID := modeltesting.GenModelUUID(c)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: modelUUID.String(),
		},
		Owner: "tlm",
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
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID:           modelUUID,
		AgentVersion: jujuversion.Current,
	}

	activated := false
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}

	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.modelDetailService.EXPECT().CreateModelForVersion(
		gomock.Any(),
		controllerUUID,
		jujuversion.Current,
		agentbinary.AgentStreamTesting,
	).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
			config.AgentStreamKey:  "testing",
		},
		Cloud:       "aws",
		CloudRegion: "region1",
		Owner:       "tlm",
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: "tlm",
		Cloud: "aws",
		Name:  "my-credential",
	})

	importOp := &importModelOperation{
		userService:             i.userService,
		modelImportService:      i.modelImportService,
		controllerConfigService: i.controllerConfigService,
		modelDetailServiceFunc:  func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
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
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID:           modelUUID,
		AgentVersion: jujuversion.Current,
	}

	var activated bool
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.modelDetailService.EXPECT().CreateModelForVersion(
		gomock.Any(), controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))
	i.modelImportService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coremodel.UUID, options ...model.DeleteModelOption) error {
		opts := model.DefaultDeleteModelOptions()
		for _, fn := range options {
			fn(opts)
		}
		c.Assert(opts.DeleteDB(), jc.IsTrue)
		return nil
	})
	i.modelDetailService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "aws",
		CloudRegion: "region1",
		Owner:       "tlm",
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: "tlm",
		Cloud: "aws",
		Name:  "my-credential",
	})

	importOp := &importModelOperation{
		userService:             i.userService,
		modelImportService:      i.modelImportService,
		controllerConfigService: i.controllerConfigService,
		modelDetailServiceFunc:  func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the model info is
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
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID:           modelUUID,
		AgentVersion: jujuversion.Current,
	}

	activated := false
	activator := func(_ context.Context) error {
		activated = true
		return nil
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.modelDetailService.EXPECT().CreateModelForVersion(
		gomock.Any(), controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))
	i.modelImportService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).Return(modelerrors.NotFound)
	i.modelDetailService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "aws",
		CloudRegion: "region1",
		Owner:       "tlm",
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: "tlm",
		Cloud: "aws",
		Name:  "my-credential",
	})

	importOp := &importModelOperation{
		userService:             i.userService,
		modelImportService:      i.modelImportService,
		controllerConfigService: i.controllerConfigService,
		modelDetailServiceFunc:  func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the model info is
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
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:  "test-model",
			Owner: userUUID,
		},
		ID:           modelUUID,
		AgentVersion: jujuversion.Current,
	}
	controllerUUID, err := uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(activator, nil)
	i.modelDetailService.EXPECT().CreateModelForVersion(
		gomock.Any(), controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))
	i.modelImportService.EXPECT().DeleteModel(gomock.Any(), modelUUID, gomock.Any()).Return(nil)
	i.modelDetailService.EXPECT().DeleteModel(gomock.Any()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey:         "test-model",
			config.UUIDKey:         modelUUID.String(),
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		Cloud:       "aws",
		CloudRegion: "region1",
		Owner:       "tlm",
		Type:        coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: "tlm",
		Cloud: "aws",
		Name:  "my-credential",
	})

	importOp := &importModelOperation{
		userService:             i.userService,
		modelImportService:      i.modelImportService,
		controllerConfigService: i.controllerConfigService,
		modelDetailServiceFunc:  func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(context.Background(), modelmigration.NewScope(nil, nil, nil), model)
	c.Check(err, gc.ErrorMatches, `.*boom.*`)

	// TODO (stickupkid): This is incorrect until the model info is
	// correctly saved.
	c.Check(activated, jc.IsTrue)
}

// TestImportModelConstraintsNoOperations asserts that if no constraints are set
// on the model's description we don't try and subsequently set constraints for
// the model on the service.
func (i *importSuite) TestImportModelConstraintsNoOperations(c *gc.C) {
	defer i.setupMocks(c).Finish()

	newUUID := modeltesting.GenModelUUID(c)
	importOp := importModelConstraintsOperation{
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	err := importOp.Execute(context.Background(), model)
	c.Check(err, jc.ErrorIsNil)

	model = description.NewModel(description.ModelArgs{
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	model.SetConstraints(description.ConstraintsArgs{})
	err = importOp.Execute(context.Background(), model)
	c.Check(err, jc.ErrorIsNil)
}

// TestImportModelConstraints is asserting the happy path of setting constraints
// from the description package on to the imported model via the service.
func (i *importSuite) TestImportModelConstraints(c *gc.C) {
	defer i.setupMocks(c).Finish()

	newUUID := modeltesting.GenModelUUID(c)
	importOp := importModelConstraintsOperation{
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	i.modelDetailService.EXPECT().SetModelConstraints(gomock.Any(), coreconstraints.Value{
		Arch:             ptr("arm64"),
		AllocatePublicIP: ptr(true),
		Spaces:           ptr([]string{"space1", "space2"}),
	})

	model := description.NewModel(description.ModelArgs{
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	model.SetConstraints(description.ConstraintsArgs{
		Architecture:     "arm64",
		AllocatePublicIP: true,
		Spaces:           []string{"space1", "space2"},
	})
	err := importOp.Execute(context.Background(), model)
	c.Check(err, jc.ErrorIsNil)
}
