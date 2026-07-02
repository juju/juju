// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coremodel "github.com/juju/juju/core/model"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/deployment/charm/repository"
	"github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type deployRepositorySuite struct {
	baseSuite
}

func TestDeployRepositorySuite(t *testing.T) {
	tc.Run(t, &deployRepositorySuite{})
}

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
		{Name: "resource1", Origin: resource.OriginStore, Revision: new(2)},
		{Name: "resource2", Origin: resource.OriginStore, Revision: new(3)},
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
		{Name: "override-revision-to-2", Origin: resource.OriginStore, Revision: new(2)},
		{Name: "no-override", Origin: resource.OriginStore, Revision: new(1)},
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

func (s *deployRepositorySuite) TestModelTypeMismatchWarningsK8sCharmOnMachineModel(c *tc.C) {
	v := deployFromRepositoryValidator{
		modelInfo: coremodel.ModelInfo{Type: coremodel.IAAS, Name: "machinemodel"},
		logger:    loggertesting.WrapCheckLog(c),
	}
	meta := &charm.Meta{Name: "redis-k8s", Containers: map[string]charm.Container{"redis": {}}}
	warnings := v.modelTypeMismatchWarnings(c.Context(), meta)

	c.Assert(warnings, tc.HasLen, 1)
	c.Check(warnings[0], tc.Equals,
		`"redis-k8s" is a Kubernetes charm (it declares containers) but "machinemodel" is a machine (IAAS) model; its workload will not run`)
}

func (s *deployRepositorySuite) TestModelTypeMismatchWarningsNilMeta(c *tc.C) {
	v := deployFromRepositoryValidator{
		modelInfo: coremodel.ModelInfo{Type: coremodel.CAAS, Name: "k8smodel"},
		logger:    loggertesting.WrapCheckLog(c),
	}
	c.Check(v.modelTypeMismatchWarnings(c.Context(), nil), tc.HasLen, 0)
}

func (s *deployRepositorySuite) TestModelTypeMismatchWarningsConsistent(c *tc.C) {
	v := deployFromRepositoryValidator{
		modelInfo: coremodel.ModelInfo{Type: coremodel.CAAS, Name: "k8smodel"},
		logger:    loggertesting.WrapCheckLog(c),
	}
	// A sidecar charm on a Kubernetes model is consistent: no warning.
	meta := &charm.Meta{Name: "redis-k8s", Containers: map[string]charm.Container{"redis": {}}}
	c.Check(v.modelTypeMismatchWarnings(c.Context(), meta), tc.HasLen, 0)
}

// TestDeployFromRepositoryReturnsWarningsOnError checks that advisory warnings
// (e.g. a charm/model-type mismatch) are returned to the client in the result
// Info even when the deploy is rejected with errors, so they reach the terminal.
func (s *deployRepositorySuite) TestDeployFromRepositoryReturnsWarningsOnError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectAnyChangeOrRemoval()
	s.newCAASAPI(c)

	info := params.DeployFromRepositoryInfo{Warnings: []string{"a charm/model mismatch warning"}}
	s.deployFromRepo.EXPECT().DeployFromRepository(gomock.Any(), gomock.Any()).
		Return(info, nil, []error{errors.New("rejected")})

	res, err := s.api.DeployFromRepository(c.Context(), params.DeployFromRepositoryArgs{
		Args: []params.DeployFromRepositoryArg{{CharmName: "x"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Errors, tc.HasLen, 1)
	c.Check(res.Results[0].Info.Warnings, tc.DeepEquals, []string{"a charm/model mismatch warning"})
}

// stubValidator returns preset results, letting us drive the inner
// DeployFromRepositoryAPI.DeployFromRepository directly.
type stubValidator struct {
	dt   deployTemplate
	errs []error
}

func (v stubValidator) ValidateArg(context.Context, params.DeployFromRepositoryArg) (deployTemplate, []error) {
	return v.dt, v.errs
}

// TestDeployFromRepositoryWarningsSurviveValidationError checks the inner
// early-return path: when ValidateArg fails, the warnings accumulated on the
// deployTemplate are still returned in the result Info.
func (s *deployRepositorySuite) TestDeployFromRepositoryWarningsSurviveValidationError(c *tc.C) {
	api := &DeployFromRepositoryAPI{
		validator: stubValidator{
			dt:   deployTemplate{warnings: []string{"a charm/model mismatch warning"}},
			errs: []error{errors.New("validation failed")},
		},
		logger: loggertesting.WrapCheckLog(c),
	}

	info, _, errs := api.DeployFromRepository(c.Context(), params.DeployFromRepositoryArg{})

	c.Assert(errs, tc.HasLen, 1)
	c.Check(info.Warnings, tc.DeepEquals, []string{"a charm/model mismatch warning"})
}
