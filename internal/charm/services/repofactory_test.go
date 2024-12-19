// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm/repository"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ = gc.Suite(&repoFactoryTestSuite{})

type repoFactoryTestSuite struct {
	testing.IsolationSuite

	modelConfigService *MockModelConfigService
	repoFactory        corecharm.RepositoryFactory
}

func (s *repoFactoryTestSuite) TestGetCharmHubRepository(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: "foo",
		config.TypeKey: "IAAS",
		config.UUIDKey: "d0d2dad4-b899-405d-b8f7-52d0f9bbe24d",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelCfg, nil)

	repo, err := s.repoFactory.GetCharmRepository(context.Background(), corecharm.CharmHub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.FitsTypeOf, new(repository.CharmHubRepository), gc.Commentf("expected to get a CharmHubRepository instance"))
}

func (s *repoFactoryTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelConfigService = NewMockModelConfigService(ctrl)

	s.repoFactory = NewCharmRepoFactory(CharmRepoFactoryConfig{
		Logger:             loggertesting.WrapCheckLog(c),
		ModelConfigService: s.modelConfigService,
	})
	return ctrl
}
