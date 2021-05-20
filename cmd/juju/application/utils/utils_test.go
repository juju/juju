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

	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/application/utils/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/resource"
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
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		return map[string]charmresource.Meta{
			"redis-image":    {Name: "redis-image", Type: charmresource.TypeContainerImage},
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
		}, ``
	})

	// switching to local charm and only upgrade resources provided.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		delete(cliResources, "redis-image")
		return map[string]charmresource.Meta{
			"snappass-image": {Name: "snappass-image", Type: charmresource.TypeContainerImage},
			"test-file":      {Name: "test-file", Type: charmresource.TypeFile, Path: "test.txt"},
		}, ``
	})

	// switching to a local charm, new resources provided will be uploaded.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
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

	// switching to ch charm, new empty resources will be uploaded.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
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

	// switching to local charm, new empty resources will be error out.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string) {
		c.Assert(newCharmURL.Schema, gc.Equals, charm.Local.String())
		resourcesInMetadata["test-file22"] = charmresource.Meta{Name: "test-file22", Type: charmresource.TypeFile, Path: "test22.txt"}
		return nil, `new resource "test-file22" was missing, please provide it via --resource`
	})

	// switching to ch charm, empty resource will be upgraded if the existing resource origin was not OriginUpload.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
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

	// switching to ch charm and empty resource will NOT be upgraded if the existing resource origin was OriginUpload.
	s.assertGetUpgradeResources(c, func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
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

func (s *utilsResourceSuite) assertGetUpgradeResources(
	c *gc.C,
	f func(
		newCharmURL *charm.URL,
		cliResources map[string]string,
		resourcesInController []resource.ApplicationResources,
		resourcesInMetadata map[string]charmresource.Meta,
	) (map[string]charmresource.Meta, string),
) {
	defer s.setupMocks(c).Finish()

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
	r1 := resource.Resource{}
	r1.Name = "redis-image"
	r2 := resource.Resource{}
	r2.Name = "snappass-image"
	r3 := resource.Resource{}
	r3.Name = "test-file"
	resourcesInController := []resource.ApplicationResources{
		{
			Resources: []resource.Resource{
				r1, r2, r3,
			},
		},
	}

	expected, errString := f(newCharmURL, cliResources, resourcesInController, resourcesInMetadata)
	s.resourceFacade.EXPECT().ListResources([]string{"snappass-test"}).Return(resourcesInController, nil)
	filtered, err := utils.GetUpgradeResources(
		newCharmURL, s.resourceFacade, "snappass-test", cliResources, resourcesInMetadata,
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
