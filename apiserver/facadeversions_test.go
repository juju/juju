// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/testing"
)

type facadeVersionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&facadeVersionSuite{})

func (s *facadeVersionSuite) TestFacadeVersionsMatchServerVersions(c *gc.C) {
	// The client side code doesn't want to directly import the server side
	// code just to list out what versions are available. However, we do
	// want to make sure that the two sides are kept in sync.
	clientFacadeNames := set.NewStrings()
	for name, version := range api.FacadeVersions {
		clientFacadeNames.Add(name)
		// All versions should now be non-zero.
		c.Check(version, jc.GreaterThan, 0)
	}
	allServerFacades := apiserver.AllFacades().List()
	serverFacadeNames := set.NewStrings()
	serverFacadeBestVersions := make(map[string]int, len(allServerFacades))
	for _, facade := range allServerFacades {
		serverFacadeNames.Add(facade.Name)
		serverFacadeBestVersions[facade.Name] = facade.Versions[len(facade.Versions)-1]
	}
	// First check that both sides know about all the same versions
	c.Check(serverFacadeNames.Difference(clientFacadeNames).SortedValues(), gc.HasLen, 0)
	c.Check(clientFacadeNames.Difference(serverFacadeNames).SortedValues(), gc.HasLen, 0)
	// Next check that the best versions match
	c.Check(api.FacadeVersions, jc.DeepEquals, serverFacadeBestVersions)
}
