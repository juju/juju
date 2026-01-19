// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	modelmigrationtesting "github.com/juju/juju/core/modelmigration/testing"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	modelImportService *MockModelImportService
	modelDetailService *MockModelDetailService
	userService        *MockUserService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelImportService = NewMockModelImportService(ctrl)
	s.modelDetailService = NewMockModelDetailService(ctrl)
	s.userService = NewMockUserService(ctrl)

	return ctrl
}

// TestModelMetadataInvalid tests that if we don't pass good values in model
// config for model name and uuid we get back an error that satisfies
// [errors.NotValid]
func (i *importSuite) TestModelMetadataInvalid(c *tc.C) {
	importOp := importModelOperation{}

	// model name not defined
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.UUIDKey: "test",
		},
	})
	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	// model name of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: 10,
			config.UUIDKey: "test",
		},
	})
	err = importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	// uuid not defined
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
		},
	})
	err = importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	// uuid of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: 11,
		},
	})
	err = importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestModelOwnerNoExist is asserting that if we try and import a model where
// the owner does not exist we get back a [usererrors.NotFound] error.
func (i *importSuite) TestModelOwnerNoExist(c *tc.C) {
	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(coreuser.User{}, usererrors.UserNotFound)

	importOp := importModelOperation{
		modelImportService: i.modelImportService,
		userService:        i.userService,
	}

	modelUUID := tc.Must0(c, coremodel.NewUUID)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: modelUUID.String(),
		},
		Owner: "tlm",
	})
	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, usererrors.UserNotFound)
}

// TestModelCreate is asserting the happy path of importing a model from
// description and the values that go into creating the model in the various
// model services.
//
// Specifically with this test we want to assert the following behaviours:
// - If no agent stream is specified in the model's config then a default stream
// of [agentbinary.AgentStreamReleased] is used.
func (i *importSuite) TestModelCreate(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	args := model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:       "test-model",
			Qualifier:  "tlm",
			AdminUsers: []coreuser.UUID{userUUID},
		},
		UUID: modelUUID,
	}

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(nil)
	i.modelDetailService.EXPECT().CreateImportingModelWithAgentVersionStream(
		gomock.Any(),
		jujuversion.Current,
		// This is important as we want to see when no agent stream has been set
		// in the imported model's config that a default value of
		// [agentbinary.AgentStreamReleased] is chosen.
		agentbinary.AgentStreamReleased,
	).Return(nil)

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
		userService:            i.userService,
		modelImportService:     i.modelImportService,
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, nil, nil, "deadbeef", tc.Must0(c, coremodel.NewUUID)),
		model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestModelCreateWithAgentStream is asserting the happy path of importing a
// model from description. Specifically with this test we want to see that if
// the model being imported has their agent stream set in model config that this
// value is used when creating the model.
func (i *importSuite) TestModelCreateWithAgentStream(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	args := model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:       "test-model",
			Qualifier:  "tlm",
			AdminUsers: []coreuser.UUID{userUUID},
		},
		UUID: modelUUID,
	}

	c.Assert(err, tc.ErrorIsNil)

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(nil)
	i.modelDetailService.EXPECT().CreateImportingModelWithAgentVersionStream(
		gomock.Any(),
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
		userService:            i.userService,
		modelImportService:     i.modelImportService,
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, nil, nil, "deadbeef", tc.Must0(c, coremodel.NewUUID)),
		model)
	c.Assert(err, tc.ErrorIsNil)
}

func (i *importSuite) TestModelCreateRollbacksOnFailure(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	args := model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:       "test-model",
			Qualifier:  "tlm",
			AdminUsers: []coreuser.UUID{userUUID},
		},
		UUID: modelUUID,
	}

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(nil)
	i.modelDetailService.EXPECT().CreateImportingModelWithAgentVersionStream(
		gomock.Any(), jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))

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
		userService:            i.userService,
		modelImportService:     i.modelImportService,
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, nil, nil, "deadbeef", tc.Must0(c, coremodel.NewUUID)),
		model)
	c.Check(err, tc.ErrorMatches, `.*boom.*`)
}

func (i *importSuite) TestModelCreateRollbacksOnFailureIgnoreNotFoundModel(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	args := model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:       "test-model",
			Qualifier:  "tlm",
			AdminUsers: []coreuser.UUID{userUUID},
		},
		UUID: modelUUID,
	}

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(nil)
	i.modelDetailService.EXPECT().CreateImportingModelWithAgentVersionStream(
		gomock.Any(), jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))

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
		userService:            i.userService,
		modelImportService:     i.modelImportService,
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, nil, nil, "deadbeef", tc.Must0(c, coremodel.NewUUID)),
		model)
	c.Check(err, tc.ErrorMatches, `.*boom.*`)
}

func (i *importSuite) TestModelCreateRollbacksOnFailureIgnoreNotFoundReadOnlyModel(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "tlm")).Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	args := model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "region1",
			Credential: credential.Key{
				Name:  "my-credential",
				Owner: usertesting.GenNewName(c, "tlm"),
				Cloud: "aws",
			},
			Name:       "test-model",
			Qualifier:  "tlm",
			AdminUsers: []coreuser.UUID{userUUID},
		},
		UUID: modelUUID,
	}

	i.modelImportService.EXPECT().ImportModel(gomock.Any(), args).Return(nil)
	i.modelDetailService.EXPECT().CreateImportingModelWithAgentVersionStream(
		gomock.Any(), jujuversion.Current, agentbinary.AgentStreamReleased,
	).Return(errors.New("boom"))

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
		userService:            i.userService,
		modelImportService:     i.modelImportService,
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	coordinator := modelmigration.NewCoordinator(
		loggertesting.WrapCheckLog(c),
		modelmigrationtesting.IgnoredSetupOperation(importOp),
	)
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, nil, nil, "deadbeef", tc.Must0(c, coremodel.NewUUID)), model)
	c.Check(err, tc.ErrorMatches, `.*boom.*`)
}

// TestImportModelConstraintsNoOperations asserts that if no constraints are set
// on the model's description we don't try and subsequently set constraints for
// the model on the service.
func (i *importSuite) TestImportModelConstraintsNoOperations(c *tc.C) {
	defer i.setupMocks(c).Finish()

	newUUID := tc.Must0(c, coremodel.NewUUID)
	importOp := importModelConstraintsOperation{
		modelDetailServiceFunc: func(_ coremodel.UUID) ModelDetailService { return i.modelDetailService },
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	err := importOp.Execute(c.Context(), model)
	c.Check(err, tc.ErrorIsNil)

	model = description.NewModel(description.ModelArgs{
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	model.SetConstraints(description.ConstraintsArgs{})
	err = importOp.Execute(c.Context(), model)
	c.Check(err, tc.ErrorIsNil)
}

// TestImportModelConstraints is asserting the happy path of setting constraints
// from the description package on to the imported model via the service.
func (i *importSuite) TestImportModelConstraints(c *tc.C) {
	defer i.setupMocks(c).Finish()

	newUUID := tc.Must0(c, coremodel.NewUUID)
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
	err := importOp.Execute(c.Context(), model)
	c.Check(err, tc.ErrorIsNil)
}

func (i *importSuite) TestModelActivate(c *tc.C) {
	defer i.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)

	i.modelImportService.EXPECT().ActivateModel(gomock.Any(), modelUUID).Return(nil)

	importOp := &importModelActivatorOperation{
		modelImportService: i.modelImportService,
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: modelUUID.String(),
		},
	})

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
