// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/environs/config"
	jujuversion "github.com/juju/juju/version"
)

type importSuite struct {
	importService *MockImportService
	userService   *MockUserService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)
	s.userService = NewMockUserService(ctrl)

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
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	// model name of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: 10,
			config.UUIDKey: "test",
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	// uuid not defined
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	// uuid of wrong type
	model = description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: 11,
		},
	})
	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

// TestModelOwnerNoExist is asserting that if we try and import a model where
// the owner does not exist we get back a [usererrors.NotFound] error.
func (i *importSuite) TestModelOwnerNoExist(c *gc.C) {
	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), "tlm").Return(coreuser.User{}, usererrors.NotFound)

	importOp := importOperation{
		service:     i.importService,
		userService: i.userService,
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
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (i *importSuite) TestModelCreate(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	userUUID, err := coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	defer i.setupMocks(c).Finish()
	i.userService.EXPECT().GetUserByName(gomock.Any(), "tlm").Return(
		coreuser.User{
			UUID: userUUID,
		},
		nil,
	)

	i.importService.EXPECT().CreateModel(gomock.Any(),
		model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        "AWS",
			CloudRegion:  "region1",
			Credential: credential.ID{
				Name:  "my-credential",
				Owner: "tlm",
				Cloud: "AWS",
			},
			Name:  "test-model",
			Owner: userUUID,
			UUID:  modelUUID,
			Type:  coremodel.CAAS,
		},
	).Return(modelUUID, nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			config.NameKey: "test-model",
			config.UUIDKey: modelUUID.String(),
		},
		Cloud:              "AWS",
		CloudRegion:        "region1",
		Owner:              names.NewUserTag("tlm"),
		LatestToolsVersion: jujuversion.Current,
		Type:               coremodel.CAAS.String(),
	})

	model.SetCloudCredential(description.CloudCredentialArgs{
		Owner: names.NewUserTag("tlm"),
		Cloud: names.NewCloudTag("AWS"),
		Name:  "my-credential",
	})

	importOp := importOperation{
		userService: i.userService,
		service:     i.importService,
	}

	err = importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
