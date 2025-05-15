// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"errors"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

type deployRepositorySuite struct {
	baseSuite
}

var _ = tc.Suite(&deployRepositorySuite{})

func (s *deployRepositorySuite) TestResolveResourcesNoResourcesOverride(c *tc.C) {
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
	result, resourceToUpload, err := validator.resolveResources(
		c.Context(),
		charmURL,
		origin,
		map[string]string{},
		resMeta,
	)
	c.Assert(err, tc.IsNil, tc.Commentf("(Act) unexpected error occurred"))

	// Assert
	c.Check(result, tc.DeepEquals, expectedResult, tc.Commentf("(Assert) expected result did not match"))
	c.Check(resourceToUpload, tc.IsNil, tc.Commentf("(Assert) expected resourcesToUpload did not match"))
}

func (s *deployRepositorySuite) TestResolveResourcesWithResourcesWithOverride(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	resMeta := map[string]resource.Meta{
		"override-revision-to-2":         {Name: "override-revision-to-2", Type: resource.TypeFile},
		"no-override":                    {Name: "no-override", Type: resource.TypeFile},
		"override-revision-to-file":      {Name: "override-revision-to-file", Type: resource.TypeFile},
		"override-revision-to-container": {Name: "override-revision-to-container", Type: resource.TypeContainerImage},
	}
	deployResArg := map[string]string{
		"override-revision-to-2":         "2",
		"override-revision-to-file":      "./toad.txt",
		"override-revision-to-container": "public.repo.com/a/b:latest",
	}
	mockRepoExpectedInput := []resource.Resource{
		{Meta: resource.Meta{Name: "override-revision-to-2", Type: resource.TypeFile}, Origin: resource.OriginStore, Revision: 2},
		{Meta: resource.Meta{Name: "no-override", Type: resource.TypeFile}, Origin: resource.OriginStore, Revision: -1},
		{Meta: resource.Meta{Name: "override-revision-to-file", Type: resource.TypeFile}, Origin: resource.OriginUpload, Revision: -1},
		{Meta: resource.Meta{Name: "override-revision-to-container", Type: resource.TypeContainerImage}, Origin: resource.OriginUpload, Revision: -1},
	}
	mockRepoResult := []resource.Resource{
		{Meta: resource.Meta{Name: "override-revision-to-2"}, Origin: resource.OriginStore, Revision: 2},
		{Meta: resource.Meta{Name: "no-override"}, Origin: resource.OriginStore, Revision: 1},
		{Meta: resource.Meta{Name: "override-revision-to-file"}, Origin: resource.OriginUpload, Revision: -1},
		{Meta: resource.Meta{Name: "override-revision-to-container"}, Origin: resource.OriginUpload, Revision: -1},
	}
	expectedResult := applicationservice.ResolvedResources{
		{Name: "override-revision-to-2", Origin: resource.OriginStore, Revision: ptr(2)},
		{Name: "no-override", Origin: resource.OriginStore, Revision: ptr(1)},
		{Name: "override-revision-to-file", Origin: resource.OriginUpload, Revision: nil},
		{Name: "override-revision-to-container", Origin: resource.OriginUpload, Revision: nil},
	}
	expectedResourcesToUpload := []*params.PendingResourceUpload{
		{Name: "override-revision-to-file", Filename: "./toad.txt", Type: "file"},
		{Name: "override-revision-to-container", Filename: "public.repo.com/a/b:latest", Type: "oci-image"},
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
	result, resourcesToUpload, err := validator.resolveResources(
		c.Context(),
		charmURL,
		origin,
		deployResArg,
		resMeta,
	)
	c.Assert(err, tc.IsNil, tc.Commentf("(Act) unexpected error occurred"))

	// Assert
	c.Check(result, tc.DeepEquals, expectedResult, tc.Commentf("(Assert) expected result did not match"))
	c.Check(resourcesToUpload, tc.SameContents, expectedResourcesToUpload, tc.Commentf("(Assert) expected resourceToUpload did not match"))
}

func (s *deployRepositorySuite) TestResolveResourcesWithResourcesErrorWhileCharmRepositoryResolve(c *tc.C) {
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
	_, _, err := validator.resolveResources(c.Context(), charmURL, origin, map[string]string{}, resMeta)

	// Assert
	c.Check(err, tc.ErrorIs, mockRepoError,
		tc.Commentf("(Assert) should return the same error as returned when resolving resources on charm repository"))
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
