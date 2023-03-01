// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/application"
	apicharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/application/utils/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/resources"
)

type utilsSuite struct{}

var _ = gc.Suite(&utilsSuite{})

func (s *utilsSuite) TestParsePlacement(c *gc.C) {
	obtained, err := utils.ParsePlacement("lxd:1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, jc.DeepEquals, instance.Placement{Scope: "lxd", Directive: "1"})

}

func (s *utilsSuite) TestGetFlags(c *gc.C) {
	flagSet := gnuflag.NewFlagSet("testing", gnuflag.ContinueOnError)
	flagSet.Bool("debug", true, "debug")
	flagSet.String("to", "", "to")
	flagSet.String("m", "default", "model")
	err := flagSet.Set("to", "lxd")
	c.Assert(err, jc.ErrorIsNil)
	obtained := utils.GetFlags(flagSet, []string{"to", "force"})
	c.Assert(obtained, gc.DeepEquals, []string{"--to"})
}

type utilsResourceSuite struct {
	charmClient    *mocks.MockCharmClient
	resourceFacade *mocks.MockResourceLister
}

var _ = gc.Suite(&utilsResourceSuite{})

func (s *utilsResourceSuite) TestGetMetaResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.expectCharmInfo(curl.String())

	obtained, err := utils.GetMetaResources(curl, s.charmClient)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, map[string]charmresource.Meta{
		"test": {Name: "Testme"}})
}

func (s *utilsResourceSuite) TestGetUpgradeResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to charm and all resources provided will be uploaded.
	s.assertGetUpgradeResources(c, nil, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		return map[string]charmresource.Meta{
			"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
		}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesLocalCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to local charm and only upgrade resources provided.
	s.assertGetUpgradeResources(c, nil, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		delete(cliResources, "redis-image")
		return map[string]charmresource.Meta{
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
		}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesLocalCharmNewResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to a local charm, new resources provided will be uploaded.
	s.assertGetUpgradeResources(c, nil, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		resourcesInMetadata["test-file22"] = charmresource.Meta{Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"}
		cliResources["test-file22"] = "./test-file22.txt"
		return map[string]charmresource.Meta{
			"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
			"test-file22":    {Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"},
		}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesCHCharmNewEmptyRes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to ch charm, new empty resources will be uploaded.
	s.assertGetUpgradeResources(c, nil, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		newCharmURL.Schema = charm.CharmHub.String()
		resourcesInMetadata["test-file22"] = charmresource.Meta{Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"}
		return map[string]charmresource.Meta{
			"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
			"test-file22":    {Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"},
		}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesLocalCharmError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to local charm, new empty resources will be error out.
	s.assertGetUpgradeResources(c, nil, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		resourcesInMetadata["test-file22"] = charmresource.Meta{Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"}
		return nil, `new resource "test-file22" was missing, please provide it via --resource`
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesNotOriginUpload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to ch charm, empty resource will be upgraded if the existing resource origin was not OriginUpload.
	s.assertGetUpgradeResources(c, nil,
		func(
			newCharmURL *charm.URL,
			cliResources map[string]string,
			resourcesInController []resources.ApplicationResources,
			resourcesInMetadata map[string]charmresource.Meta,
		) (map[string]charmresource.Meta, string) {
			newCharmURL.Schema = charm.CharmHub.String()
			delete(cliResources, "redis-image")

			redis := resourcesInController[0].Resources[0]
			redis.Origin = charmresource.OriginStore
			resourcesInController[0].Resources[0] = redis

			return map[string]charmresource.Meta{
				"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
				"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
				"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
			}, ``
		})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesOriginUpload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// switching to ch charm and empty resource will NOT be upgraded if the existing resource origin was OriginUpload.
	s.assertGetUpgradeResources(c, nil,
		func(
			newCharmURL *charm.URL,
			cliResources map[string]string,
			resourcesInController []resources.ApplicationResources,
			resourcesInMetadata map[string]charmresource.Meta,
		) (map[string]charmresource.Meta, string) {
			newCharmURL.Schema = charm.CharmHub.String()
			delete(cliResources, "redis-image")

			redis := resourcesInController[0].Resources[0]
			redis.Origin = charmresource.OriginUpload
			resourcesInController[0].Resources[0] = redis

			return map[string]charmresource.Meta{
				"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
				"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
			}, ``
		})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesRepositoryNoChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	redis := charmresource.Resource{}
	redis.Name = "redis-image"
	redis.Origin = charmresource.OriginStore
	redis.Revision = 5
	snappass := charmresource.Resource{}
	snappass.Name = "snappass-image"
	snappass.Origin = charmresource.OriginStore
	snappass.Revision = 3
	testfile := charmresource.Resource{}
	testfile.Name = "test-file"
	testfile.Origin = charmresource.OriginStore
	testfile.Revision = 2
	availableCharmResources := []charmresource.Resource{
		redis, snappass, testfile,
	}

	// switching to charm and all resources provided will be uploaded.
	s.assertGetUpgradeResources(c, availableCharmResources, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		newCharmURL.Schema = charm.CharmHub.String()
		delete(cliResources, "redis-image")
		delete(cliResources, "snappass-image")
		delete(cliResources, "test-file")
		redis := resourcesInController[0].Resources[0]
		redis.Origin = charmresource.OriginStore
		redis.Revision = 5
		resourcesInController[0].Resources[0] = redis
		snappass := resourcesInController[0].Resources[1]
		snappass.Origin = charmresource.OriginStore
		snappass.Revision = 3
		resourcesInController[0].Resources[1] = snappass
		testfile := resourcesInController[0].Resources[2]
		testfile.Origin = charmresource.OriginStore
		testfile.Revision = 2
		resourcesInController[0].Resources[2] = testfile

		return map[string]charmresource.Meta{}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesRepositoryChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// No resources specified on the CLI - but the repository has new
	// resource revisions to use.

	redis := charmresource.Resource{}
	redis.Name = "redis-image"
	redis.Revision = 7 // This resource has a new revision
	snappass := charmresource.Resource{}
	snappass.Name = "snappass-image"
	snappass.Revision = 3
	testfile := charmresource.Resource{}
	testfile.Name = "test-file"
	testfile.Revision = 2
	availableCharmResources := []charmresource.Resource{
		redis, snappass, testfile,
	}

	// switching to charm and all resources provided will be uploaded.
	s.assertGetUpgradeResources(c, availableCharmResources, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		newCharmURL.Schema = charm.CharmHub.String()
		delete(cliResources, "redis-image")
		delete(cliResources, "snappass-image")
		delete(cliResources, "test-file")
		redis := resourcesInController[0].Resources[0]
		redis.Origin = charmresource.OriginStore
		redis.Revision = 5
		resourcesInController[0].Resources[0] = redis
		snappass := resourcesInController[0].Resources[1]
		snappass.Origin = charmresource.OriginStore
		snappass.Revision = 3
		resourcesInController[0].Resources[1] = snappass
		testfile := resourcesInController[0].Resources[2]
		testfile.Origin = charmresource.OriginStore
		testfile.Revision = 2
		resourcesInController[0].Resources[2] = testfile

		return map[string]charmresource.Meta{"redis-image": {Name: "redis-image", Type: charmresource.TypeContainerImage}}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesRepositoryCLIRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// No change in the resource revisions in the repository, but a different
	// resource revision is specified on the cli.

	redis := charmresource.Resource{}
	redis.Name = "redis-image"
	redis.Origin = charmresource.OriginStore
	redis.Revision = 5
	snappass := charmresource.Resource{}
	snappass.Name = "snappass-image"
	snappass.Origin = charmresource.OriginStore
	snappass.Revision = 3
	testfile := charmresource.Resource{}
	testfile.Name = "test-file"
	testfile.Origin = charmresource.OriginStore
	testfile.Revision = 2
	availableCharmResources := []charmresource.Resource{
		redis, snappass, testfile,
	}

	// switching to charm and all resources provided will be uploaded.
	s.assertGetUpgradeResources(c, availableCharmResources, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		newCharmURL.Schema = charm.CharmHub.String()
		delete(cliResources, "redis-image")
		delete(cliResources, "snappass-image")

		cliResources["test-file"] = "42"

		redis := resourcesInController[0].Resources[0]
		redis.Origin = charmresource.OriginStore
		redis.Revision = 5
		resourcesInController[0].Resources[0] = redis
		snappass := resourcesInController[0].Resources[1]
		snappass.Origin = charmresource.OriginStore
		snappass.Revision = 3
		resourcesInController[0].Resources[1] = snappass
		testfile := resourcesInController[0].Resources[2]
		testfile.Origin = charmresource.OriginStore
		testfile.Revision = 2
		resourcesInController[0].Resources[2] = testfile

		return map[string]charmresource.Meta{"test-file": {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"}}, ``
	})
}

func (s *utilsResourceSuite) TestGetUpgradeResourcesRepositoryCLIRevisionAlreadyUsed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// No change in the resource revisions in the repository, but a different
	// resource revision is specified on the cli.

	redis := charmresource.Resource{}
	redis.Name = "redis-image"
	redis.Origin = charmresource.OriginStore
	redis.Revision = 5
	snappass := charmresource.Resource{}
	snappass.Name = "snappass-image"
	snappass.Origin = charmresource.OriginStore
	snappass.Revision = 3
	testfile := charmresource.Resource{}
	testfile.Name = "test-file"
	testfile.Origin = charmresource.OriginStore
	testfile.Revision = 2
	availableCharmResources := []charmresource.Resource{
		redis, snappass, testfile,
	}

	// switching to charm and all resources provided will be uploaded.
	s.assertGetUpgradeResources(c, availableCharmResources, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		newCharmURL.Schema = charm.CharmHub.String()
		delete(cliResources, "redis-image")
		delete(cliResources, "snappass-image")

		cliResources["test-file"] = "42"

		redis := resourcesInController[0].Resources[0]
		redis.Origin = charmresource.OriginStore
		redis.Revision = 5
		resourcesInController[0].Resources[0] = redis
		snappass := resourcesInController[0].Resources[1]
		snappass.Origin = charmresource.OriginStore
		snappass.Revision = 3
		resourcesInController[0].Resources[1] = snappass
		testfile := resourcesInController[0].Resources[2]
		testfile.Origin = charmresource.OriginStore
		testfile.Revision = 42
		resourcesInController[0].Resources[2] = testfile

		return map[string]charmresource.Meta{}, ``
	})
}

func (s *utilsResourceSuite) assertGetUpgradeResources(
	c *gc.C,
	charmResources []charmresource.Resource,
	getExpectedMeta func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resources.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string),
) {
	newCharmURL := &charm.URL{Schema: "local", Name: "snappass-test", Revision: 0, Series: "focal"}
	cliResources := map[string]string{
		"snappass-image": "snappass-test",
		"redis-image":    "redis",
		"test-file":      "./test-file.txt",
	}
	resourcesInMetadata := map[string]charmresource.Meta{
		"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
		"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
		"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
	}
	r1 := resources.Resource{}
	r1.Name = "redis-image"
	r2 := resources.Resource{}
	r2.Name = "snappass-image"
	r3 := resources.Resource{}
	r3.Name = "test-file"
	resourcesInController := []resources.ApplicationResources{
		{
			Resources: []resources.Resource{
				r1, r2, r3,
			},
		},
	}

	expected, errString := getExpectedMeta(newCharmURL, cliResources, resourcesInController, resourcesInMetadata)
	s.resourceFacade.EXPECT().ListResources([]string{"snappass-test"}).Return(resourcesInController, nil)
	s.charmClient.EXPECT().ListCharmResources(gomock.AssignableToTypeOf(&charm.URL{}), gomock.AssignableToTypeOf(apicharm.Origin{})).Return(charmResources, nil)
	charmID := application.CharmID{
		URL:    newCharmURL,
		Origin: apicharm.Origin{},
	}
	filtered, err := utils.GetUpgradeResources(
		charmID, s.charmClient, s.resourceFacade, "snappass-test", cliResources, resourcesInMetadata,
	)
	if len(errString) == 0 {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, errString)
	}
	c.Assert(filtered, gc.DeepEquals, expected)
}

func (s *utilsResourceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmClient = mocks.NewMockCharmClient(ctrl)
	s.resourceFacade = mocks.NewMockResourceLister(ctrl)
	return ctrl
}

func (s *utilsResourceSuite) expectCharmInfo(str string) {
	charmInfo := &apicommoncharms.CharmInfo{
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"test": {Name: "Testme"},
			},
		},
	}
	s.charmClient.EXPECT().CharmInfo(str).Return(charmInfo, nil)
}
