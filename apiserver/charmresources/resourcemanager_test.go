// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"fmt"
	"github.com/juju/juju/apiserver/charmresources"
	"github.com/juju/juju/apiserver/params"
	resources "github.com/juju/juju/charmresources"
)

type resourcesSuite struct {
	baseResourcesSuite
}

var _ = gc.Suite(&resourcesSuite{})

func (s *resourcesSuite) TestResourcesListEmpty(c *gc.C) {
	s.resourceManager.resourceList = func(filter resources.ResourceAttributes) ([]resources.Resource, error) {
		s.calls = append(s.calls, resourceListCall)
		return []resources.Resource{}, nil
	}

	found, err := s.api.ResourceList(params.ResourceFilterParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Resources, gc.HasLen, 0)
	expectedCalls := []string{
		resourceManagerCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)
}

func (s *resourcesSuite) TestResourcesListAll(c *gc.C) {
	now := time.Now()
	s.addResource(resources.Resource{Path: "respath", Size: 100, Created: now})
	found, err := s.api.ResourceList(params.ResourceFilterParams{})

	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		resourceManagerCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Resources, gc.HasLen, 1)
	c.Assert(found.Resources[0], jc.DeepEquals, params.ResourceMetadata{
		ResourcePath: "/blob/respath",
		Size:         100,
		Created:      now,
	})
}

func (s *resourcesSuite) TestResourcesList(c *gc.C) {
	now := time.Now()
	s.addResource(resources.Resource{Path: "respath", Size: 100, Created: now})
	s.addResource(resources.Resource{Path: "s/trusty/another", Size: 200, Created: now})
	s.addResource(resources.Resource{Path: "s/precise/another", Size: 300, Created: now})
	found, err := s.api.ResourceList(params.ResourceFilterParams{
		Resources: []params.ResourceParams{
			{Series: "trusty"},
			{PathName: "respath"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		resourceManagerCall,
		resourceListCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(found.Resources, gc.HasLen, 2)
	c.Assert(found.Resources, jc.SameContents, []params.ResourceMetadata{
		{
			ResourcePath: "/blob/respath",
			Size:         100,
			Created:      now,
		}, {
			ResourcePath: "/blob/s/trusty/another",
			Size:         200,
			Created:      now,
		},
	})
}

func (s *resourcesSuite) TestResourcesListError(c *gc.C) {
	msg := "list test error"
	s.resourceManager.resourceList = func(filter resources.ResourceAttributes) ([]resources.Resource, error) {
		s.calls = append(s.calls, resourceListCall)
		return nil, errors.New(msg)
	}

	found, err := s.api.ResourceList(params.ResourceFilterParams{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)

	expectedCalls := []string{
		resourceManagerCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(found.Resources, gc.HasLen, 0)
}

func (s *resourcesSuite) TestResourcesDeleteError(c *gc.C) {
	s.addResource(resources.Resource{Path: "respath", Size: 100})
	msg := "delete test error"
	s.resourceManager.resourceDelete = func(path string) error {
		s.calls = append(s.calls, resourceDeleteCall)
		return errors.New(msg)
	}

	result, err := s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{{PathName: "respath"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		getBlockForTypeCall,
		getBlockForTypeCall,
		resourceManagerCall,
		resourceListCall,
		resourceDeleteCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error.Error(), gc.Equals, msg)
}

func (s *resourcesSuite) TestResourcesNotFoundDeleteError(c *gc.C) {
	result, err := s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{{PathName: "foo"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		getBlockForTypeCall,
		getBlockForTypeCall,
		resourceManagerCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
}

func (s *resourcesSuite) TestNoResourcesDeleteError(c *gc.C) {
	result, err := s.api.ResourceDelete(params.ResourceFilterParams{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "no resources specified to delete")

	expectedCalls := []string{
		getBlockForTypeCall,
		getBlockForTypeCall,
	}
	s.assertCalls(c, expectedCalls)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *resourcesSuite) TestResourceDeleteBlockedAllChanges(c *gc.C) {
	s.blockAllChanges(c, "TestResourceDeleteBlocked")

	_, err := s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{{PathName: "foo"}},
	})
	s.assertBlocked(c, err, "TestResourceDeleteBlocked")
	expectedCalls := []string{
		getBlockForTypeCall,
		getBlockForTypeCall,
	}
	s.assertCalls(c, expectedCalls)
}

func (s *resourcesSuite) TestResourceDeleteBlockedDeletes(c *gc.C) {
	s.blockRemoveObject(c, "TestResourceDeleteBlockedDeletes")

	_, err := s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{{PathName: "foo"}},
	})
	s.assertBlocked(c, err, "TestResourceDeleteBlockedDeletes")
	expectedCalls := []string{
		getBlockForTypeCall,
	}
	s.assertCalls(c, expectedCalls)
}

func (s *resourcesSuite) TestResourcesDeleteNotEnvOwner(c *gc.C) {
	s.environOwner = "foo"
	var err error
	s.api, err = charmresources.CreateAPI(s.state, s.apiResources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{{PathName: "foo"}},
	})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "permission denied")
}

func (s *resourcesSuite) TestResourcesDelete(c *gc.C) {
	now := time.Now()
	s.addResource(resources.Resource{Path: "respath", Size: 100, Created: now})
	s.addResource(resources.Resource{Path: "s/trusty/another", Size: 200, Created: now})
	s.addResource(resources.Resource{Path: "s/precise/another", Size: 300, Created: now})
	result, err := s.api.ResourceDelete(params.ResourceFilterParams{
		Resources: []params.ResourceParams{
			{Series: "trusty"},
			{PathName: "respath"},
			{PathName: "notfound"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := []string{
		getBlockForTypeCall,
		getBlockForTypeCall,
		resourceManagerCall,
		resourceListCall,
		resourceDeleteCall,
		resourceListCall,
	}
	s.assertCalls(c, expectedCalls)

	c.Assert(result.Results, gc.HasLen, 3)
	var expectedErrors = make([]string, len(result.Results))
	for i, err := range result.Results {
		if err.Error == nil {
			continue
		}
		expectedErrors[i] = result.Results[i].Error.Message
	}
	c.Assert(expectedErrors, gc.DeepEquals, []string{
		"resource path name cannot be empty", "", "resource /blob/notfound not found",
	})

	fmt.Println(s.resources)

	c.Assert(s.resources, gc.HasLen, 2)
	var expectedPaths []string
	for p, _ := range s.resources {
		expectedPaths = append(expectedPaths, p)
	}
	c.Assert(expectedPaths, jc.SameContents, []string{
		"/blob/s/trusty/another",
		"/blob/s/precise/another",
	})
}
