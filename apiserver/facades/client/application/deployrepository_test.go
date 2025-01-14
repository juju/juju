// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"errors"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charm/resource"
)

type deployRepositorySuite struct {
	baseSuite
}

var _ = gc.Suite(&deployRepositorySuite{})

func (s *deployRepositorySuite) TestResolveResourcesNoResourcesOverride(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	resMeta := map[string]resource.Meta{
		"resource1": {Name: "resource1"},
		"resource2": {Name: "resource2"},
	}
	mockRepoExpectedInput := []resource.Resource{
		{Meta: resource.Meta{Name: "resource1"}, Origin: resource.OriginStore, Revision: -1},
		{Meta: resource.Meta{Name: "resource2"}, Origin: resource.OriginStore, Revision: -1},
	}
	mockRepoResult := []resource.Resource{
		{Meta: resource.Meta{Name: "resource1"}, Origin: resource.OriginStore, Revision: 2},
		{Meta: resource.Meta{Name: "resource2"}, Origin: resource.OriginStore, Revision: 3},
	}
	expectedResult := applicationservice.ResolvedResources{
		{Name: "resource1", Origin: resource.OriginStore, Revision: ptr(2)},
		{Name: "resource2", Origin: resource.OriginStore, Revision: ptr(3)},
	}
	charmURL := charm.MustParseURL("ch:ubuntu-0")
	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
	}
	s.charmRepository.EXPECT().ResolveResources(gomock.Any(), gomock.InAnyOrder(mockRepoExpectedInput), corecharm.CharmID{
		URL:    charmURL,
		Origin: origin,
	}).Return(mockRepoResult, nil)
	validator := s.expectValidator()

	// Act
	result, err := validator.resolveResources(context.Background(), charmURL, origin, map[string]string{}, resMeta)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) unexpected error occurred"))

	// Assert
	c.Check(result, gc.DeepEquals, expectedResult, gc.Commentf("(Assert) expected result did not match"))
}

func (s *deployRepositorySuite) TestResolveResourcesWithResourcesWithOverride(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	resMeta := map[string]resource.Meta{
		"override-revision-to-2":    {Name: "override-revision-to-2"},
		"no-override":               {Name: "no-override"},
		"override-revision-to-file": {Name: "override-revision-to-file"},
	}
	deployResArg := map[string]string{
		"override-revision-to-2":    "2",
		"override-revision-to-file": "./toad.txt",
	}
	mockRepoExpectedInput := []resource.Resource{
		{Meta: resource.Meta{Name: "override-revision-to-2"}, Origin: resource.OriginStore, Revision: 2},
		{Meta: resource.Meta{Name: "no-override"}, Origin: resource.OriginStore, Revision: -1},
		{Meta: resource.Meta{Name: "override-revision-to-file"}, Origin: resource.OriginUpload, Revision: -1},
	}
	mockRepoResult := []resource.Resource{
		{Meta: resource.Meta{Name: "override-revision-to-2"}, Origin: resource.OriginStore, Revision: 2},
		{Meta: resource.Meta{Name: "no-override"}, Origin: resource.OriginStore, Revision: 1},
		{Meta: resource.Meta{Name: "override-revision-to-file"}, Origin: resource.OriginUpload, Revision: -1},
	}
	expectedResult := applicationservice.ResolvedResources{
		{Name: "override-revision-to-2", Origin: resource.OriginStore, Revision: ptr(2)},
		{Name: "no-override", Origin: resource.OriginStore, Revision: ptr(1)},
		{Name: "override-revision-to-file", Origin: resource.OriginUpload, Revision: nil},
	}
	charmURL := charm.MustParseURL("ch:ubuntu-0")
	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
	}

	s.charmRepository.EXPECT().ResolveResources(gomock.Any(), gomock.InAnyOrder(mockRepoExpectedInput), corecharm.CharmID{
		URL:    charmURL,
		Origin: origin,
	}).Return(mockRepoResult, nil)
	validator := s.expectValidator()

	// Act
	result, err := validator.resolveResources(context.Background(), charmURL, origin, deployResArg, resMeta)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) unexpected error occurred"))

	// Assert
	c.Check(result, gc.DeepEquals, expectedResult, gc.Commentf("(Assert) expected result did not match"))
}

func (s *deployRepositorySuite) TestResolveResourcesWithResourcesErrorWhileCharmRepositoryResolve(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	resMeta := map[string]resource.Meta{
		"no-override": {Name: "no-override"},
	}
	mockRepoError := errors.New("not supported: test")
	charmURL := charm.MustParseURL("ch:ubuntu-0")
	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
	}

	s.charmRepository.EXPECT().ResolveResources(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, mockRepoError)
	validator := s.expectValidator()

	// Act
	_, err := validator.resolveResources(context.Background(), charmURL, origin, map[string]string{}, resMeta)

	// Assert
	c.Check(err, jc.ErrorIs, mockRepoError,
		gc.Commentf("(Assert) should return the same error as returned when resolving resources on charm repository"))
}

// expectValidator sets up a mock deployFromRepositoryValidator with predefined expectations for testing purposes.
func (s *deployRepositorySuite) expectValidator() deployFromRepositoryValidator {
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(&config.Config{}, nil)
	validator := deployFromRepositoryValidator{
		modelConfigService: s.modelConfigService,
		newCharmHubRepository: func(repositoryConfig repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return s.charmRepository, nil
		},
	}
	return validator
}
