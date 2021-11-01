// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"net/http"

	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/charm/repository"
	"github.com/juju/juju/environs/config"
)

var _ = gc.Suite(&repoFactoryTestSuite{})

type repoFactoryTestSuite struct {
	testing.IsolationSuite

	stateBackend *MockStateBackend
	modelBackend *MockModelBackend
	repoFactory  corecharm.RepositoryFactory
}

func (s *repoFactoryTestSuite) TestGetCharmStoreRepository(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateBackend.EXPECT().ControllerConfig().Return(
		controller.Config{
			controller.CharmStoreURL: "https://blobs4u.charmstore.com",
		},
		nil,
	)

	repo, err := s.repoFactory.GetCharmRepository(corecharm.CharmStore)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.FitsTypeOf, new(repository.CharmStoreRepository), gc.Commentf("expected to get a CharmStoreRepository instance"))
}

func (s *repoFactoryTestSuite) TestGetCharmHubRepository(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: "foo",
		config.TypeKey: "IAAS",
		config.UUIDKey: "d0d2dad4-b899-405d-b8f7-52d0f9bbe24d",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.modelBackend.EXPECT().Config().Return(modelCfg, nil)

	repo, err := s.repoFactory.GetCharmRepository(corecharm.CharmHub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo, gc.FitsTypeOf, new(repository.CharmHubRepository), gc.Commentf("expected to get a CharmHubRepository instance"))
}

func (s *repoFactoryTestSuite) TestGetCharmRepositoryMemoization(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: "foo",
		config.TypeKey: "IAAS",
		config.UUIDKey: "d0d2dad4-b899-405d-b8f7-52d0f9bbe24d",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.modelBackend.EXPECT().Config().Return(modelCfg, nil)

	repo1, err := s.repoFactory.GetCharmRepository(corecharm.CharmHub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo1, gc.FitsTypeOf, new(repository.CharmHubRepository), gc.Commentf("expected to get a CharmHubRepository instance"))

	repo2, err := s.repoFactory.GetCharmRepository(corecharm.CharmHub)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo2, gc.FitsTypeOf, new(repository.CharmHubRepository), gc.Commentf("expected to get a CharmHubRepository instance"))

	// Note: we are comparing pointer values here hence the use of gc.Equals.
	c.Assert(repo1, gc.Equals, repo2, gc.Commentf("expected to get memoized instance for CharmHub repository"))
}

func (s *repoFactoryTestSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateBackend = NewMockStateBackend(ctrl)
	s.modelBackend = NewMockModelBackend(ctrl)

	s.repoFactory = NewCharmRepoFactory(CharmRepoFactoryConfig{
		Logger:       loggo.GetLogger("test"),
		StateBackend: s.stateBackend,
		ModelBackend: s.modelBackend,
		Transport:    http.DefaultClient,
	})
	return ctrl
}
